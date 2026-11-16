package workspace

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func registeredPath(value string) (string, error) {
	value = strings.TrimPrefix(value, "/workspace/")
	if value == "" || value == "." || strings.HasPrefix(value, "/") || path.Clean(value) != value || strings.ContainsAny(value, "\x00\\") || strings.HasPrefix(value, "../") {
		return "", toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	return value, nil
}

// A new saved byte identity replaces only the mutable Workspace registration.
// Sealed tool artifacts/deliveries remain immutable; old file IDs become unavailable.
func registerSavedFile(ctx context.Context, q *db.Queries, p toolruntime.Principal, w db.Workspace,
	filePath string, size int64, hash, media string) (db.WorkspaceFile, error) {
	relative, err := registeredPath(filePath)
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	old, err := q.GetWorkspaceFileByPath(ctx, db.GetWorkspaceFileByPathParams{WorkspaceID: w.ID, EnterpriseID: p.EnterpriseID, Path: relative})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.WorkspaceFile{}, err
	}
	if err == nil {
		if old.ContentHash == hash && old.ByteSize == size {
			return old, nil
		}
		count, e := q.RetireWorkspaceFileRegistration(ctx, db.RetireWorkspaceFileRegistrationParams{ID: old.ID, EnterpriseID: p.EnterpriseID, WorkspaceID: w.ID, ContentHash: old.ContentHash})
		if e != nil {
			return db.WorkspaceFile{}, e
		}
		if count != 1 {
			return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
		}
	}
	if media == "" {
		media = publicationMediaType(relative, path.Base(relative), "")
	}
	file, err := q.CreateWorkspaceFile(ctx, db.CreateWorkspaceFileParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID,
		WorkspaceID: w.ID, Name: path.Base(relative), Path: relative, ByteSize: size, ContentHash: hash, MediaType: media, SourceResultRefs: w.SourceResultRefs})
	if err != nil {
		return file, err
	}
	_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, Type: "workspace_file_added", ActorType: "service",
		Payload: map[string]any{"file": fileView(file), "registration": "saved_working_copy"}, Classification: "internal"})
	return file, err
}

func (service Service) recordSavedFile(ctx context.Context, p toolruntime.Principal, access *Access, filePath string, size int64, hash string) (db.WorkspaceFile, error) {
	var file db.WorkspaceFile
	err := service.publishFile(ctx, p, access, func(q *db.Queries, w db.Workspace) error {
		var err error
		file, err = registerSavedFile(ctx, q, p, w, filePath, size, hash, "")
		return err
	})
	return file, err
}
