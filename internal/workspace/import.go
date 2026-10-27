package workspace

import (
	"bytes"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspacefs"
)

func (service Service) ImportResult(ctx context.Context, p toolruntime.Principal, runID uuid.UUID, ref, filePath string) (db.WorkspaceFile, error) {
	filePath, err := workspacefs.Normalize(filePath)
	if err != nil {
		return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	source, err := (conversation.Service{Store: service.Store, Objects: service.Objects}).ReadToolResult(ctx, p.EnterpriseID, p.UserID, ref)
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	if !source.Artifact.ConversationID.Valid || source.Artifact.ConversationID.UUID != p.ConversationID {
		return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	}
	access, err := waitQueryImportAccess(ctx, func() (*Access, error) {
		// Revalidate while waiting for another reader/writer to release the lease.
		if err := service.validateSources(ctx, p, []string{ref}); err != nil {
			return nil, err
		}
		return service.access(ctx, p, false, uuid.Nil, accessScope{RunID: runID})
	})
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	defer access.Close()
	file, err := service.importQueryFile(ctx, p, ref, filePath, bytes.NewReader(source.Artifact.Content), int64(len(source.Artifact.Content)), fmt.Sprintf("%x", source.Artifact.ContentHash), access)
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
