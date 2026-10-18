package workspace

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func validRemoteArtifact(a remoteexecution.OutputArtifact, limit int64) bool {
	clean, err := workspacefs.Normalize(a.SourcePath)
	digest, hashErr := hex.DecodeString(a.ContentHash)
	if err != nil || clean != a.SourcePath || len(a.SourcePath)+len(remoteexecution.ResultsDirectory)+1 > 4096 || strings.HasPrefix(a.SourcePath, "/") || a.ByteSize < 0 || a.ByteSize > limit || hashErr != nil || len(digest) != 32 || strings.ToLower(a.ContentHash) != a.ContentHash {
		return false
	}
	for _, part := range strings.Split(a.SourcePath, "/") {
		if len(part) > 255 {
			return false
		}
	}
	return a.ID != uuid.Nil && a.RequestID != uuid.Nil && a.RunID != uuid.Nil && a.StepID != uuid.Nil && a.SessionID != uuid.Nil && !a.CollectedAt.IsZero()
}

// ImportRemoteOutput owns the editable business copy and its trusted mapping.
// The raw remote artifact remains immutable in the remote execution ledger.
func (s Service) ImportRemoteOutput(ctx context.Context, p toolruntime.Principal, artifact remoteexecution.OutputArtifact, reader io.Reader) (file db.WorkspaceFile, failure error) {
	defer func() { failure = fileError(failure) }()
	if s.ExternalSource == nil || !validRemoteArtifact(artifact, s.Config.MaxFileBytes) {
		return file, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	access, err := s.externalImportAccess(ctx, p, artifact.RunID, artifact.SourceRef(), true)
	if err != nil {
		return file, err
	}
	defer access.Close()
	file, err = s.importRemoteArtifact(ctx, p, artifact, reader, access)
	if err != nil {
		return file, err
	}
	return file, access.Close()
}

func (s Service) importRemoteArtifact(ctx context.Context, p toolruntime.Principal, artifact remoteexecution.OutputArtifact, reader io.Reader, access *Access) (db.WorkspaceFile, error) {
	if err := access.registerSource(ctx, artifact.SourceRef()); err != nil {
		return db.WorkspaceFile{}, err
	}
	mapping, err := s.reserveResultArtifact(ctx, p, artifact, access)
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	file, err := s.importQueryFile(ctx, p, artifact.SourceRef(), mapping.Path, reader, artifact.ByteSize, artifact.ContentHash, access)
	if err != nil {
		return file, err
	}
	err = s.publishFile(ctx, p, access, func(q *db.Queries, w db.Workspace) error {
		count, err := q.CompleteWorkspaceResultArtifact(ctx, db.CompleteWorkspaceResultArtifactParams{WorkspaceID: w.ID, EnterpriseID: p.EnterpriseID, ArtifactID: artifact.ID, WorkspaceFileID: uuid.NullUUID{UUID: file.ID, Valid: true}})
		if err != nil {
			return err
		}
		if count != 1 {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
		}
		return nil
	})
	if err != nil {
		return file, err
	}
	return file, s.publishResultsMetadata(ctx, p, access)
}

func (s Service) reserveResultArtifact(ctx context.Context, p toolruntime.Principal, artifact remoteexecution.OutputArtifact, access *Access) (db.WorkspaceResultArtifact, error) {
	existing, err := s.Store.Queries.GetWorkspaceResultArtifact(ctx, db.GetWorkspaceResultArtifactParams{WorkspaceID: access.Workspace.ID, EnterpriseID: p.EnterpriseID, ArtifactID: artifact.ID})
	if err == nil {
		var known remoteexecution.OutputArtifact
		if json.Unmarshal(existing.Metadata, &known) != nil || remoteexecution.Digest(known) != remoteexecution.Digest(artifact) {
			return existing, toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return existing, err
	}
	desired := remoteexecution.ResultsDirectory + "/" + artifact.SourcePath
	for n := 1; n <= 4096; n++ {
		candidate := resultCollisionPath(desired, n)
		if len(candidate) > 4096 {
			return existing, toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
		}
		if candidate == remoteexecution.ResultsMetadataPath {
			continue
		}
		occupied, err := s.resultPathOccupied(ctx, p, access, candidate)
		if err != nil {
			return existing, err
		}
		if occupied {
			continue
		}
		encoded, err := json.Marshal(artifact)
		if err != nil {
			return existing, err
		}
		err = s.publishFile(ctx, p, access, func(q *db.Queries, w db.Workspace) error {
			existing, err = q.ReserveWorkspaceResultArtifact(ctx, db.ReserveWorkspaceResultArtifactParams{WorkspaceID: w.ID, EnterpriseID: p.EnterpriseID, ArtifactID: artifact.ID, Path: candidate, Metadata: encoded})
			return err
		})
		return existing, err
	}
	return existing, toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
}

func resultCollisionPath(desired string, n int) string {
	if n == 1 {
		return desired
	}
	base := path.Base(desired)
	ext := path.Ext(base)
	if base == ext || len(ext) > 128 {
		ext = ""
	}
	stem, suffix := strings.TrimSuffix(base, ext), fmt.Sprintf(" (%d)", n)
	for len(stem)+len(suffix)+len(ext) > 255 {
		_, width := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-width]
	}
	return path.Join(path.Dir(desired), stem+suffix+ext)
}

func (s Service) resultPathOccupied(ctx context.Context, p toolruntime.Principal, access *Access, candidate string) (bool, error) {
	_, err := s.Store.Queries.GetWorkspaceResultArtifactByPath(ctx, db.GetWorkspaceResultArtifactByPathParams{WorkspaceID: access.Workspace.ID, EnterpriseID: p.EnterpriseID, Path: candidate})
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	_, err = s.Store.Queries.GetWorkspaceFileByPath(ctx, db.GetWorkspaceFileByPathParams{WorkspaceID: access.Workspace.ID, EnterpriseID: p.EnterpriseID, Path: candidate})
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	reader, _, err := access.IO.Read(access.Context, candidate, 0, 1)
	if err == nil {
		return true, reader.Close()
	}
	// A directory at the requested filename also requires a distinct filename.
	if status.Code(err) == codes.InvalidArgument {
		return true, nil
	}
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}
