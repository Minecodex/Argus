package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (service Service) CreateUpload(ctx context.Context, p toolruntime.Principal, name string, size int64, key string) (db.WorkspaceUpload, error) {
	if !safeFileName(name) || size < 0 || size > service.Config.MaxFileBytes {
		return db.WorkspaceUpload{}, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	workspace, err := service.ensure(ctx, p, accessScope{})
	if err != nil {
		return db.WorkspaceUpload{}, err
	}
	input := struct {
		Name        string
		Size        int64
		WorkspaceID uuid.UUID
	}{name, size, workspace.ID}
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", p.UserID.String(), "workspace.upload.create", key, input, 201, func(q *db.Queries) (db.WorkspaceUpload, error) {
		if _, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID}); err != nil {
			return db.WorkspaceUpload{}, err
		}
		current, err := q.LockWorkspace(ctx, db.LockWorkspaceParams{ID: workspace.ID, EnterpriseID: p.EnterpriseID})
		if err != nil || current.Status != "ready" {
			return db.WorkspaceUpload{}, toolruntime.Error{Kind: "WORKSPACE_UNAVAILABLE"}
		}
		id := uuid.New()
		return q.CreateWorkspaceUpload(ctx, db.CreateWorkspaceUploadParams{ID: id, EnterpriseID: p.EnterpriseID, WorkspaceID: workspace.ID, OwnerUserID: p.UserID, Name: name,
			Path: "uploads/" + id.String() + "/" + name, ExpectedBytes: size, RequestID: key})
	})
}

func (service Service) Upload(ctx context.Context, p toolruntime.Principal, id uuid.UUID, reader io.Reader) (value db.WorkspaceFile, failure error) {
	defer func() { failure = fileError(failure) }()
	if err := service.Authorize(ctx, p); err != nil {
		return db.WorkspaceFile{}, err
	}
	old, err := service.Store.Queries.GetWorkspaceUpload(ctx, db.GetWorkspaceUploadParams{ID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	workspace, err := service.Get(ctx, p)
	if err != nil || old.WorkspaceID != workspace.ID {
		return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	if old.Status == "complete" && old.FileID.Valid {
		file, err := service.Store.Queries.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: old.FileID.UUID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return db.WorkspaceFile{}, err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, io.LimitReader(reader, service.Config.MaxFileBytes+1))
		if err != nil || size != file.ByteSize || hex.EncodeToString(hash.Sum(nil)) != file.ContentHash {
			return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_UPLOAD_CONFLICT"}
		}
		return file, nil
	}
	upload, err := service.Store.Queries.ClaimWorkspaceUpload(ctx, db.ClaimWorkspaceUploadParams{ID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
	if err != nil {
		return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_UPLOAD_CONFLICT"}
	}
	access, err := service.access(ctx, p, false, uuid.Nil, accessScope{WorkspaceID: upload.WorkspaceID})
	if err != nil {
		service.failUpload(ctx, p.EnterpriseID, id)
		return db.WorkspaceFile{}, err
	}
	defer access.Close()
	info, err := access.IO.Write(access.Context, upload.Path, io.LimitReader(reader, service.Config.MaxFileBytes+1), upload.ExpectedBytes, false)
	if err != nil {
		service.failUpload(ctx, p.EnterpriseID, id)
		return db.WorkspaceFile{}, err
	}
	var file db.WorkspaceFile
	if err := access.checkAuthorization(ctx); err != nil {
		service.failUpload(ctx, p.EnterpriseID, id)
		return file, err
	}
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID}); err != nil {
			return err
		}
		current, err := q.LockWorkspace(ctx, db.LockWorkspaceParams{ID: workspace.ID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return err
		}
		if current.Status != "ready" || current.FenceToken != access.Workspace.FenceToken || current.LeaseOwner.String != access.owner {
			return toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
		}
		media := mime.TypeByExtension(path.Ext(upload.Name))
		if media == "" {
			media = "application/octet-stream"
		}
		file, err = q.CreateWorkspaceFile(ctx, db.CreateWorkspaceFileParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, WorkspaceID: workspace.ID, ConversationID: p.ConversationID,
			Name: upload.Name, Path: upload.Path, ByteSize: info.ByteSize, ContentHash: info.ContentHash, MediaType: media, SourceResultRefs: []string{}})
		if err != nil {
			return err
		}
		count, err := q.FinishWorkspaceUpload(ctx, db.FinishWorkspaceUploadParams{ID: id, EnterpriseID: p.EnterpriseID, Status: "complete", FileID: uuid.NullUUID{UUID: file.ID, Valid: true}})
		if err != nil {
			return err
		}
		if count != 1 {
			return toolruntime.Error{Kind: "WORKSPACE_UPLOAD_CONFLICT"}
		}
		_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, Type: "workspace_file_added", ActorType: "user", ActorID: p.UserID.String(),
			Payload: map[string]any{"file": fileView(file)}, Classification: "internal"})
		return err
	})
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	return file, access.Close()
}

func (service Service) failUpload(ctx context.Context, enterprise, id uuid.UUID) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _ = service.Store.Queries.FinishWorkspaceUpload(cleanup, db.FinishWorkspaceUploadParams{ID: id, EnterpriseID: enterprise, Status: "failed", ErrorCode: pgtype.Text{String: "WORKSPACE_UPLOAD_FAILED", Valid: true}})
}

func (service Service) ListFiles(ctx context.Context, p toolruntime.Principal) ([]db.WorkspaceFile, error) {
	workspace, err := service.Get(ctx, p)
	if err != nil {
		return nil, err
	}
	return service.Store.Queries.ListWorkspaceFiles(ctx, db.ListWorkspaceFilesParams{WorkspaceID: workspace.ID, EnterpriseID: p.EnterpriseID})
}

type FileReader struct {
	io.ReadCloser
	closeAccess func() error
	authorize   func() error
}

func (reader FileReader) Read(data []byte) (int, error) {
	if reader.authorize != nil {
		if err := reader.authorize(); err != nil {
			return 0, err
		}
	}
	return reader.ReadCloser.Read(data)
}

func (reader FileReader) Close() error {
	err := reader.ReadCloser.Close()
	if reader.closeAccess != nil {
		if closeErr := reader.closeAccess(); err == nil {
			err = closeErr
		}
	}
	return err
}

func (service Service) ReadFile(ctx context.Context, p toolruntime.Principal, id uuid.UUID, offset, length int64) (returned io.ReadCloser, returnedSize int64, returnedName string, failure error) {
	defer func() { failure = fileError(failure) }()
	file, err := service.Store.Queries.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return nil, 0, "", err
	}
	if file.ConversationID != p.ConversationID {
		return nil, 0, "", toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	}
	if err := service.validateSources(ctx, p, file.SourceResultRefs); err != nil {
		return nil, 0, "", err
	}
	access, err := service.access(ctx, p, false, uuid.Nil, accessScope{WorkspaceID: file.WorkspaceID})
	if err != nil {
		return nil, 0, "", err
	}
	if access.Workspace.ID != file.WorkspaceID {
		access.Close()
		return nil, 0, "", toolruntime.Error{Kind: "WORKSPACE_FILE_NOT_FOUND"}
	}
	reader, size, err := access.IO.Read(access.Context, file.Path, offset, length)
	if err != nil {
		access.Close()
		return nil, 0, "", err
	}
	return FileReader{ReadCloser: reader, closeAccess: access.Close, authorize: func() error { return access.checkAuthorization(ctx) }}, size, file.Name, nil
}

func (service Service) Publish(ctx context.Context, p toolruntime.Principal, runID uuid.UUID, filePath, name, media string) (value db.FileDelivery, failure error) {
	defer func() { failure = fileError(failure) }()
	if name == "" {
		name = path.Base(filePath)
	}
	if !safeFileName(name) {
		return db.FileDelivery{}, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	if media == "" {
		media = mime.TypeByExtension(path.Ext(name))
	}
	if media == "" {
		media = "application/octet-stream"
	}
	if _, _, err := mime.ParseMediaType(media); err != nil {
		return db.FileDelivery{}, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	if service.Objects == nil {
		return db.FileDelivery{}, toolruntime.Error{Kind: "WORKSPACE_STORAGE_UNAVAILABLE"}
	}
	access, err := service.access(ctx, p, false, uuid.Nil, accessScope{RunID: runID})
	if err != nil {
		return db.FileDelivery{}, err
	}
	defer access.Close()
	sources, err := service.publicationSources(ctx, p, access.Workspace.ID)
	if err != nil {
		return db.FileDelivery{}, err
	}
	reader, size, err := access.IO.Read(access.Context, filePath, 0, 0)
	if err != nil {
		return db.FileDelivery{}, err
	}
	defer reader.Close()
	if size > service.Config.MaxFileBytes {
		return db.FileDelivery{}, toolruntime.Error{Kind: "WORKSPACE_FILE_TOO_LARGE"}
	}
	id := uuid.New()
	key := objectPrefix(access.Workspace) + "deliveries/" + id.String()
	hash := sha256.New()
	if err := service.Objects.PutFile(access.Context, key, io.TeeReader(reader, hash), size, media); err != nil {
		return db.FileDelivery{}, err
	}
	var delivery db.FileDelivery
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
		if err := access.checkAuthorization(ctx); err != nil {
			return err
		}
		delivery, err = q.CreateFileDelivery(ctx, db.CreateFileDeliveryParams{ID: id, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, WorkspaceID: current.ID, RunID: uuid.NullUUID{UUID: runID, Valid: runID != uuid.Nil},
			Name: name, MediaType: media, ByteSize: size, ContentHash: hex.EncodeToString(hash.Sum(nil)), ObjectKey: key, SourceResultRefs: sources})
		if err != nil {
			return err
		}
		_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, RunID: uuid.NullUUID{UUID: runID, Valid: runID != uuid.Nil}, Type: "artifact_published", ActorType: "service",
			Payload: map[string]any{"artifact": deliveryView(delivery)}, Classification: "internal"})
		return err
	})
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = service.Objects.DeleteFile(cleanup, key)
		return db.FileDelivery{}, err
	}
	return delivery, access.Close()
}

func (service Service) ReadDelivery(ctx context.Context, p toolruntime.Principal, id uuid.UUID, offset, length int64) (returned io.ReadCloser, value db.FileDelivery, failure error) {
	defer func() { failure = fileError(failure) }()
	if err := service.Authorize(ctx, p); err != nil {
		return nil, db.FileDelivery{}, err
	}
	file, err := service.Store.Queries.GetFileDelivery(ctx, db.GetFileDeliveryParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return nil, file, err
	}
	if file.ConversationID != p.ConversationID {
		return nil, file, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	}
	if err := service.validateSources(ctx, p, file.SourceResultRefs); err != nil {
		return nil, file, err
	}
	workspace, err := service.Get(ctx, p)
	if err != nil || workspace.ID != file.WorkspaceID || workspace.Status != "ready" {
		return nil, file, toolruntime.Error{Kind: "WORKSPACE_FILE_NOT_FOUND"}
	}
	if service.Objects == nil {
		return nil, file, toolruntime.Error{Kind: "WORKSPACE_STORAGE_UNAVAILABLE"}
	}
	reader, err := service.Objects.ReadFile(ctx, file.ObjectKey, offset, length)
	if err != nil {
		return nil, file, err
	}
	return FileReader{ReadCloser: reader, authorize: func() error {
		if err := service.Authorize(ctx, p); err != nil {
			return err
		}
		return service.validateSources(ctx, p, file.SourceResultRefs)
	}}, file, nil
}

func safeFileName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && !strings.ContainsAny(name, "/\\\r\n\x00")
}
func fileView(file db.WorkspaceFile) map[string]any {
	return map[string]any{"id": file.ID.String(), "name": file.Name, "path": "/workspace/" + file.Path, "byte_size": file.ByteSize, "content_hash": file.ContentHash, "media_type": file.MediaType, "created_at": file.CreatedAt.Time}
}
func deliveryView(file db.FileDelivery) map[string]any {
	return map[string]any{"id": file.ID.String(), "name": file.Name, "byte_size": file.ByteSize, "content_hash": file.ContentHash, "media_type": file.MediaType, "created_at": file.CreatedAt.Time}
}
