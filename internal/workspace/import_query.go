package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ImportQueryFile accepts only service-originated immutable query artifacts.
// It deliberately is not a model tool: neither bytes nor their provenance may
// be asserted by a model. All delivery and replay goes through the same lease,
// quota, atomic upload and durable directory source checks as other files.
func (service Service) ImportQueryFile(ctx context.Context, p toolruntime.Principal, runID uuid.UUID, ref, filePath string, reader io.Reader, size int64, digest string) (file db.WorkspaceFile, failure error) {
	defer func() { failure = fileError(failure) }()
	if service.ExternalSource == nil || size < 0 || size > service.Config.MaxFileBytes || len(digest) != 64 || !strings.HasPrefix(filePath, "queries/") {
		return file, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	access, err := waitQueryImportAccess(ctx, func() (*Access, error) {
		// Each attempt revalidates the source; waiting must not preserve a grant
		// revoked while another foreground tool owns the Workspace lease.
		handled, err := service.ExternalSource(ctx, p, ref)
		if err != nil {
			return nil, err
		}
		if !handled {
			return nil, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
		}
		return service.access(ctx, p, false, uuid.Nil, accessScope{RunID: runID, BackgroundTransfer: true})
	})
	if err != nil {
		return file, err
	}
	defer access.Close()
	file, err = service.importQueryFile(ctx, p, ref, filePath, reader, size, digest, access)
	if err != nil {
		return file, err
	}
	return file, access.Close()
}

// Wait only before acquiring access, while no input bytes have been consumed.
// A short read/bash operation must not exhaust a background job's retry count.
// Actual writes, quota failures, authorization failures and lost leases are not
// replayed here. Parent cancellation and query-worker fencing remain effective.
func waitQueryImportAccess(ctx context.Context, acquire func() (*Access, error)) (*Access, error) {
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		access, err := acquire()
		var coded interface{ Code() string }
		if !errors.As(err, &coded) || coded.Code() != "WORKSPACE_BUSY" {
			return access, err
		}
		delay := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			delay.Stop()
			return nil, ctx.Err()
		case <-deadline.C:
			delay.Stop()
			return nil, err
		case <-delay.C:
		}
	}
}

func (service Service) importQueryFile(ctx context.Context, p toolruntime.Principal, ref, filePath string, reader io.Reader, size int64, digest string, access *Access) (file db.WorkspaceFile, err error) {
	if err := access.registerSource(ctx, ref); err != nil {
		return file, err
	}
	existing, e := service.Store.Queries.GetWorkspaceFileByPath(ctx, db.GetWorkspaceFileByPathParams{WorkspaceID: access.Workspace.ID, EnterpriseID: p.EnterpriseID, Path: filePath})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return file, e
	}
	if e == nil && (existing.ByteSize != size || existing.ContentHash != digest || len(existing.SourceResultRefs) != 1 || existing.SourceResultRefs[0] != ref) {
		return file, toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
	}
	// Handle a crash after the file was renamed but before metadata committed.
	// A changed Workspace copy is never silently replaced on resume.
	current, actual, readErr := access.IO.Read(access.Context, filePath, 0, 0)
	if readErr == nil {
		h := sha256.New()
		n, copyErr := io.Copy(h, io.LimitReader(current, size+1))
		closeErr := current.Close()
		if copyErr != nil {
			return file, copyErr
		}
		if closeErr != nil {
			return file, closeErr
		}
		if actual != size || n != size || hex.EncodeToString(h.Sum(nil)) != digest {
			return file, toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
		}
	} else if status.Code(readErr) == codes.NotFound {
		checked := &queryImportReader{reader: io.LimitReader(reader, size+1), digest: sha256.New(), expected: digest, size: size}
		info, err := access.IO.Write(access.Context, filePath, checked, size, false)
		if err != nil {
			return file, err
		}
		if info.ByteSize != size || info.ContentHash != digest {
			return file, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
	} else {
		return file, readErr
	}
	if err := access.checkAuthorization(ctx); err != nil {
		return file, err
	}
	if e == nil {
		return existing, nil
	}
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		c, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err != nil {
			return err
		}
		if c.Status != "active" {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
		}
		w, err := q.LockWorkspace(ctx, db.LockWorkspaceParams{ID: access.Workspace.ID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return err
		}
		if w.Status != "ready" || w.FenceToken != access.Workspace.FenceToken || w.LeaseOwner.String != access.owner || !w.LeaseUntil.Valid || !w.LeaseUntil.Time.After(time.Now()) {
			return toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
		}
		file, err = q.CreateWorkspaceFile(ctx, db.CreateWorkspaceFileParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, WorkspaceID: w.ID, ConversationID: p.ConversationID, Name: path.Base(filePath), Path: filePath, ByteSize: size, ContentHash: digest, MediaType: "application/json", SourceResultRefs: []string{ref}})
		if err != nil {
			return err
		}
		_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, Type: "workspace_file_added", ActorType: "service", Payload: map[string]any{"file": fileView(file)}, Classification: "internal"})
		return err
	})
	if err != nil {
		return file, err
	}
	return file, nil
}

type queryImportReader struct {
	reader         io.Reader
	digest         hash.Hash
	expected       string
	size, received int64
}

func (r *queryImportReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		_, _ = r.digest.Write(p[:n])
		r.received += int64(n)
	}
	if r.received > r.size || err == io.EOF && (r.received != r.size || hex.EncodeToString(r.digest.Sum(nil)) != r.expected) {
		return n, toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	return n, err
}
