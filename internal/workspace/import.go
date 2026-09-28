package workspace

import (
	"bytes"
	"context"
	"path"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (service Service) ImportResult(ctx context.Context, p toolruntime.Principal, runID uuid.UUID, ref, filePath string) (db.WorkspaceFile, error) {
	source, err := (conversation.Service{Store: service.Store, Objects: service.Objects}).ReadToolResult(ctx, p.EnterpriseID, p.UserID, ref)
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	if !source.Artifact.ConversationID.Valid || source.Artifact.ConversationID.UUID != p.ConversationID {
		return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	}
	access, err := service.access(ctx, p, false, uuid.Nil, accessScope{RunID: runID})
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	defer access.Close()
	if err := access.registerSource(ctx, ref); err != nil {
		return db.WorkspaceFile{}, err
	}
	info, err := access.IO.Write(access.Context, filePath, bytes.NewReader(source.Artifact.Content), int64(len(source.Artifact.Content)), false)
	if err != nil {
		return db.WorkspaceFile{}, fileError(err)
	}
	if err := access.checkAuthorization(ctx); err != nil {
		return db.WorkspaceFile{}, err
	}
	var file db.WorkspaceFile
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID}); err != nil {
			return err
		}
		current, err := q.LockWorkspace(ctx, db.LockWorkspaceParams{ID: access.Workspace.ID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return err
		}
		if current.Status != "ready" || current.FenceToken != access.Workspace.FenceToken {
			return toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
		}
		file, err = q.CreateWorkspaceFile(ctx, db.CreateWorkspaceFileParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, WorkspaceID: current.ID, ConversationID: p.ConversationID, Name: path.Base(info.Path), Path: info.Path, ByteSize: info.ByteSize, ContentHash: info.ContentHash, MediaType: "application/json", SourceResultRefs: []string{ref}})
		if err != nil {
			return err
		}
		_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, Type: "workspace_file_added", ActorType: "service", Payload: map[string]any{"file": fileView(file)}, Classification: "internal"})
		return err
	})
	if err != nil {
		return file, err
	}
	return file, access.Close()
}
func (service Service) validateSources(ctx context.Context, p toolruntime.Principal, refs []string) error {
	for _, ref := range refs {
		if service.ExternalSource != nil {
			handled, err := service.ExternalSource(ctx, p, ref)
			if err != nil {
				return err
			}
			if handled {
				continue
			}
		}
		if _, err := (conversation.Service{Store: service.Store}).GetToolResult(ctx, p.EnterpriseID, p.UserID, ref); err != nil {
			return err
		}
	}
	return nil
}
