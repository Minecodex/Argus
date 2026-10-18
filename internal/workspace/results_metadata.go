package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type resultMetadataEntry struct {
	remoteexecution.OutputArtifact
	Path            string    `json:"path"`
	WorkspaceFileID uuid.UUID `json:"workspace_file_id"`
	SourceRef       string    `json:"source_ref"`
}

type resultsMetadata struct {
	SchemaVersion  string                `json:"schema_version"`
	WorkspaceID    uuid.UUID             `json:"workspace_id"`
	ConversationID uuid.UUID             `json:"conversation_id"`
	Files          []resultMetadataEntry `json:"files"`
}

// Rebuild from the database, never merge facts asserted by the editable file.
func (s Service) publishResultsMetadata(ctx context.Context, p toolruntime.Principal, access *Access) error {
	rows, err := s.Store.Queries.ListWorkspaceResultArtifacts(ctx, db.ListWorkspaceResultArtifactsParams{WorkspaceID: access.Workspace.ID, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return err
	}
	manifest := resultsMetadata{SchemaVersion: "argus.workspace_results/v1", WorkspaceID: access.Workspace.ID, ConversationID: p.ConversationID, Files: []resultMetadataEntry{}}
	sources := []string{}
	for _, row := range rows {
		var artifact remoteexecution.OutputArtifact
		if json.Unmarshal(row.Metadata, &artifact) != nil || !row.WorkspaceFileID.Valid {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
		manifest.Files = append(manifest.Files, resultMetadataEntry{OutputArtifact: artifact, Path: "/workspace/" + row.Path, WorkspaceFileID: row.WorkspaceFileID.UUID, SourceRef: artifact.SourceRef()})
		if !slices.Contains(sources, artifact.SourceRef()) {
			sources = append(sources, artifact.SourceRef())
		}
	}
	sort.Strings(sources)
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if int64(len(encoded)) > min(s.Config.MaxFileBytes, 8<<20) {
		return toolruntime.Error{Kind: "WORKSPACE_FILE_TOO_LARGE"}
	}
	if err := access.checkAuthorization(ctx); err != nil {
		return err
	}
	info, err := access.IO.Write(access.Context, remoteexecution.ResultsMetadataPath, bytes.NewReader(encoded), int64(len(encoded)), true)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	if info.ByteSize != int64(len(encoded)) || info.ContentHash != digest {
		return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	if err := access.checkAuthorization(ctx); err != nil {
		return err
	}
	return s.publishFile(ctx, p, access, func(q *db.Queries, w db.Workspace) error {
		file, err := q.UpsertWorkspaceResultsMetadataFile(ctx, db.UpsertWorkspaceResultsMetadataFileParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, WorkspaceID: w.ID, ConversationID: p.ConversationID, ByteSize: info.ByteSize, ContentHash: digest, SourceResultRefs: sources})
		if err != nil {
			return err
		}
		// This event is an idempotent file projection; the stable file ID keeps
		// refreshes of the single metadata file from becoming separate files.
		_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, Type: "workspace_file_added", ActorType: "service", Payload: map[string]any{"file": fileView(file), "metadata_index": true}, Classification: "internal"})
		return err
	})
}
