package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (s Service) FreezeRemoteInputs(ctx context.Context, p toolruntime.Principal, runID, requestID uuid.UUID) ([]remoteexecution.File, error) {
	empty := []remoteexecution.File{}
	if !s.Config.Enabled {
		return empty, nil
	}
	w, err := s.Store.Queries.GetConversationWorkspace(ctx, db.GetConversationWorkspaceParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	access, err := s.access(ctx, p, false, uuid.Nil, accessScope{RunID: runID, WorkspaceID: w.ID})
	if err != nil {
		return nil, err
	}
	defer access.Close()
	listing, err := access.IO.RPC.ListFiles(access.Context, &workspacev1.ListFilesRequest{Authority: access.IO.Authority, Path: remoteexecution.InputDirectory, MaxFiles: remoteexecution.MaxFiles, MaxBytes: remoteexecution.MaxTransferBytes})
	if err != nil {
		return nil, err
	}
	result := []remoteexecution.File{}
	var total int64
	previewBudget := remoteexecution.MaxScriptBytes
	for _, file := range listing.Files {
		if err = access.checkAuthorization(ctx); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(file.Path, remoteexecution.InputDirectory+"/") {
			return nil, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
		reader, size, err := access.IO.Read(access.Context, file.Path, 0, 0)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, remoteexecution.MaxTransferBytes+1))
		closeErr := reader.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		total += int64(len(data))
		if total > remoteexecution.MaxTransferBytes || int64(len(data)) != size || size != file.ByteSize {
			return nil, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
		hash := sha256.Sum256(data)
		digest := hex.EncodeToString(hash[:])
		relative := strings.TrimPrefix(file.Path, remoteexecution.InputDirectory+"/")
		key := remoteexecution.InputObjectKey(p.EnterpriseID, requestID, relative, digest)
		if s.Objects == nil {
			return nil, remoteexecution.Error("REMOTE_TRANSFER_UNAVAILABLE")
		}
		if err = s.Objects.PutFile(access.Context, key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); err != nil {
			return nil, err
		}
		preview := ""
		if len(data) <= previewBudget && utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
			preview = string(data)
			previewBudget -= len(data)
		}
		result = append(result, remoteexecution.File{ID: uuid.NewSHA1(requestID, []byte(relative)), ContentPreview: preview, WorkspaceID: w.ID, Path: relative, Size: int64(len(data)), Hash: digest, ObjectKey: key})
	}
	if err = access.checkAuthorization(ctx); err != nil {
		return nil, err
	}
	return result, access.Close()
}
func (s Service) ValidateRemoteInputs(ctx context.Context, p toolruntime.Principal, files []remoteexecution.File) error {
	current, err := s.Get(ctx, p)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.WorkspaceID != current.ID {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
		}
	}
	return s.authorizeWorkspace(ctx, p, current)
}
