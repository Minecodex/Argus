package remoteexecution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"google.golang.org/protobuf/encoding/protojson"
)

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func safePath(p string) bool {
	return p != "" && !strings.ContainsAny(p, "\\:\x00") && p != "." && p != ".." && !strings.HasPrefix(p, "../") && !path.IsAbs(p) && path.Clean(p) == p
}
func (s Service) prepareFiles(ctx context.Context, r db.RemoteExecutionRequest, step Step, identity db.RemoteExecutionIdentity, binding *connectorv1.RemoteExecutionBatch) error {
	raw, err := s.Transport.Call(ctx, identity, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "open", SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Binding: binding}, r.ID.String()+"/open/"+step.ID.String(), 120*time.Second, nil)
	if err != nil {
		return err
	}
	var opened connectorv1.RemoteExecutionControlResult
	if protojson.Unmarshal(raw, &opened) != nil || opened.SessionGeneration != step.Generation.String() || opened.ContextRevision != binding.ContextRevision || opened.Username != identity.Username || opened.IdentitySid != identity.IdentitySid {
		return Error("REMOTE_CONTEXT_CHANGED")
	}
	if opened.RuntimeAccount == "" || opened.InterpreterVersion == "" {
		return Error("REMOTE_IDENTITY_UNAVAILABLE")
	}
	n, err := s.Store.Queries.RecordRemoteRuntimeIdentity(ctx, db.RecordRemoteRuntimeIdentityParams{ID: step.SessionID, EnterpriseID: r.EnterpriseID, Generation: step.Generation, RuntimeAccount: opened.RuntimeAccount, InterpreterVersion: opened.InterpreterVersion})
	if err != nil {
		return err
	}
	if n != 1 {
		return Error("REMOTE_CONTEXT_CHANGED")
	}
	files, err := s.Store.Queries.ListRemoteFiles(ctx, db.ListRemoteFilesParams{RequestID: r.ID, EnterpriseID: r.EnterpriseID, Direction: "input"})
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.SessionID.Valid {
			continue
		} // Per-target receipts are not new inputs.
		if err := s.uploadFrozenInput(ctx, r, step, identity, f); err != nil {
			return err
		}
	}
	return nil
}
func (s Service) collectFiles(ctx context.Context, r db.RemoteExecutionRequest, step Step, identity db.RemoteExecutionIdentity) ([]File, error) {
	raw, err := s.Transport.Call(ctx, identity, "remote_execution_file", &connectorv1.RemoteExecutionFile{SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Operation: "list_output"}, r.ID.String()+"/output-list/"+step.ID.String(), 30*time.Second, nil)
	if errorCode(err, "") == "REMOTE_CONTEXT_LOST" {
		binding := batchBinding(r, step, 1, step.ContextRevision)
		var spec Spec
		if json.Unmarshal(r.Spec, &spec) != nil {
			return nil, Error("REMOTE_PLAN_INVALID")
		}
		for n, s := range spec.Steps {
			if s.ID == step.ID {
				binding.Sequence = uint32(n + 1)
				break
			}
		}
		recovered, e := s.Transport.Call(ctx, identity, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "recover_files", SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Binding: binding}, r.ID.String()+"/recover-files/"+step.ID.String(), 30*time.Second, nil)
		var receipt connectorv1.RemoteExecutionControlResult
		if e != nil {
			return nil, e
		}
		if protojson.Unmarshal(recovered, &receipt) != nil || receipt.Status != "files_only" || receipt.SessionGeneration != step.Generation.String() {
			return nil, Error("REMOTE_COMPLETION_INVALID")
		}
		raw, err = s.Transport.Call(ctx, identity, "remote_execution_file", &connectorv1.RemoteExecutionFile{SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Operation: "list_output"}, r.ID.String()+"/output-list-restored/"+step.ID.String(), 30*time.Second, nil)
	}
	if err != nil {
		return nil, err
	}
	var listing connectorv1.RemoteExecutionFileResult
	if protojson.Unmarshal(raw, &listing) != nil || !listing.Complete || len(listing.Files) > MaxFiles {
		return nil, Error("REMOTE_OUTPUT_INTEGRITY")
	}
	files := []File{}
	var total uint64
	for _, f := range listing.Files {
		total += f.ByteSize
		if total > MaxTransferBytes || !safePath(f.Path) || len(f.Sha256) != 64 {
			return nil, Error("REMOTE_OUTPUT_INTEGRITY")
		}
		data := []byte{}
		for offset := uint64(0); offset < f.ByteSize; {
			raw, err = s.Transport.Call(ctx, identity, "remote_execution_file", &connectorv1.RemoteExecutionFile{SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Operation: "read_output", Path: f.Path, Offset: offset, ByteSize: f.ByteSize, Sha256: f.Sha256}, fmt.Sprintf("%s/output/%s/%s/%d", r.ID, step.ID, f.Path, offset), 30*time.Second, nil)
			if err != nil {
				return nil, err
			}
			var chunk connectorv1.RemoteExecutionFileResult
			if protojson.Unmarshal(raw, &chunk) != nil || len(chunk.Data) == 0 || len(chunk.Data) > ChunkBytes || chunk.Offset != offset+uint64(len(chunk.Data)) || chunk.Offset > f.ByteSize {
				return nil, Error("REMOTE_OUTPUT_INTEGRITY")
			}
			data = append(data, chunk.Data...)
			offset = chunk.Offset
		}
		if hashBytes(data) != f.Sha256 {
			return nil, Error("REMOTE_OUTPUT_INTEGRITY")
		}
		file, err := s.storeOutput(ctx, r, step, f.Path, data)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
func (s Service) storeOutput(ctx context.Context, r db.RemoteExecutionRequest, step Step, relative string, data []byte) (File, error) {
	digest := hashBytes(data)
	ref := "remote:" + r.ID.String()
	logical := step.ID.String() + "/" + relative
	key := "remote/" + r.EnterpriseID.String() + "/" + r.ID.String() + "/output/" + digest + "/" + logical
	if s.Objects == nil {
		return File{}, Error("REMOTE_TRANSFER_UNAVAILABLE")
	}
	if err := s.Objects.PutFile(ctx, key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); err != nil {
		return File{}, err
	}
	file, err := s.Store.Queries.CreateRemoteFile(ctx, db.CreateRemoteFileParams{ID: uuid.New(), EnterpriseID: r.EnterpriseID, RequestID: r.ID, SessionID: uuid.NullUUID{UUID: step.SessionID, Valid: true}, Direction: "output", Path: logical, ByteSize: int64(len(data)), ContentHash: digest, ObjectKey: key, Status: "sealed", SourceRef: ref})
	if err != nil {
		return File{}, err
	}
	if file.ContentHash != digest || file.ByteSize != int64(len(data)) {
		return File{}, Error("REMOTE_OUTPUT_INTEGRITY")
	}
	workspacePath := ResultsDirectory + "/" + relative
	if s.Files != nil {
		p, err := s.principal(ctx, r.EnterpriseID, r.OwnerUserID, r.ConversationID)
		if err != nil {
			return File{}, err
		}
		var spec Spec
		if json.Unmarshal(r.Spec, &spec) != nil {
			return File{}, Error("REMOTE_PLAN_INVALID")
		}
		artifact := OutputArtifact{ID: file.ID, RequestID: r.ID, RunID: r.RunID, StepID: step.ID, SessionID: step.SessionID, Target: step.Target, RequestTitle: spec.Title, StepDescription: step.Description, SourcePath: relative, ByteSize: file.ByteSize, ContentHash: file.ContentHash, CollectedAt: file.CreatedAt.Time.UTC()}
		imported, err := s.Files.ImportRemoteOutput(ctx, p, artifact, bytes.NewReader(data))
		if err != nil {
			return File{}, err
		}
		workspacePath = imported.Path
	}
	if _, err = s.Store.Queries.UpdateRemoteFileTransfer(ctx, db.UpdateRemoteFileTransferParams{ID: file.ID, EnterpriseID: r.EnterpriseID, TransferredBytes: int64(len(data)), Status: "complete"}); err != nil {
		return File{}, err
	}
	return File{ID: file.ID, Path: workspacePath, Size: int64(len(data)), Hash: digest, SourceRef: ref}, nil
}
func (s Service) AuthorizeSource(ctx context.Context, p toolruntime.Principal, ref string) (bool, error) {
	if !strings.HasPrefix(ref, "remote:") {
		return false, nil
	}
	id, err := uuid.Parse(strings.TrimPrefix(ref, "remote:"))
	if err != nil {
		return true, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	_, err = s.Get(ctx, p, id)
	return true, err
}
