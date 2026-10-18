package remoteexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

// Inputs are delivered once to each session in a fixed plan. A later step or
// worker retry must not silently overwrite changes made by an approved step.
func (s Service) uploadFrozenInput(ctx context.Context, r db.RemoteExecutionRequest, step Step, identity db.RemoteExecutionIdentity, f db.RemoteExecutionFile) (failure error) {
	if s.Objects == nil || !safePath(f.Path) || f.ByteSize < 0 || f.ByteSize > MaxTransferBytes {
		return Error("REMOTE_TRANSFER_UNAVAILABLE")
	}
	receipt, err := s.Store.Queries.CreateRemoteFile(ctx, db.CreateRemoteFileParams{
		ID: uuid.NewSHA1(f.ID, []byte("input/"+step.SessionID.String())), EnterpriseID: r.EnterpriseID, RequestID: r.ID,
		SessionID: uuid.NullUUID{UUID: step.SessionID, Valid: true}, Direction: "input", Path: f.Path, ByteSize: f.ByteSize,
		ContentHash: f.ContentHash, ObjectKey: f.ObjectKey, Status: "transferring", SourceRef: f.SourceRef,
	})
	if err != nil {
		return err
	}
	if receipt.ContentHash != f.ContentHash || receipt.ByteSize != f.ByteSize || receipt.ObjectKey != f.ObjectKey {
		return Error("REMOTE_INPUT_INTEGRITY")
	}
	if receipt.Status == "complete" && receipt.TransferredBytes == f.ByteSize {
		return nil
	}
	progress := receipt.TransferredBytes
	defer func() {
		if failure != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			status := "failed"
			if progress > 0 {
				status = "partial"
			}
			_, _ = s.Store.Queries.UpdateRemoteFileTransfer(cleanup, db.UpdateRemoteFileTransferParams{ID: receipt.ID, EnterpriseID: r.EnterpriseID, TransferredBytes: progress, Status: status})
		}
	}()
	reader, err := s.Objects.ReadFile(ctx, f.ObjectKey, 0, 0)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = reader.Close()
		}
	}()
	input := io.LimitReader(reader, f.ByteSize+1)
	hash := sha256.New()
	buffer := make([]byte, ChunkBytes)
	var offset int64
	for {
		n, readErr := io.ReadFull(input, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if offset+int64(n) > f.ByteSize {
			return Error("REMOTE_INPUT_INTEGRITY")
		}
		if n > 0 || offset == 0 {
			_, _ = hash.Write(buffer[:n])
			request := &connectorv1.RemoteExecutionFile{SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Operation: "write_input", Path: f.Path, Offset: uint64(offset), Data: buffer[:n], ByteSize: uint64(f.ByteSize)}
			if _, err = s.Transport.Call(ctx, identity, "remote_execution_file", request, fmt.Sprintf("%s/input/%s/%s/%d", r.ID, step.SessionID, f.ID, offset), 30*time.Second, nil); err != nil {
				return err
			}
			offset += int64(n)
			progress = max(progress, offset)
			if _, err = s.Store.Queries.UpdateRemoteFileTransfer(ctx, db.UpdateRemoteFileTransferParams{ID: receipt.ID, EnterpriseID: r.EnterpriseID, TransferredBytes: offset, Status: "transferring"}); err != nil {
				return err
			}
		}
		if readErr != nil {
			break
		}
	}
	if err = reader.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if offset != f.ByteSize || hex.EncodeToString(hash.Sum(nil)) != f.ContentHash {
		return Error("REMOTE_INPUT_INTEGRITY")
	}
	raw, err := s.Transport.Call(ctx, identity, "remote_execution_file", &connectorv1.RemoteExecutionFile{SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), RequestId: r.ID.String(), Operation: "finish_input", Path: f.Path, ByteSize: uint64(f.ByteSize), Sha256: f.ContentHash}, r.ID.String()+"/input-finish/"+step.SessionID.String()+"/"+f.ID.String(), 30*time.Second, nil)
	if err != nil {
		return err
	}
	var finished connectorv1.RemoteExecutionFileResult
	if protojson.Unmarshal(raw, &finished) != nil || !finished.Complete {
		return Error("REMOTE_INPUT_INTEGRITY")
	}
	n, err := s.Store.Queries.UpdateRemoteFileTransfer(ctx, db.UpdateRemoteFileTransferParams{ID: receipt.ID, EnterpriseID: r.EnterpriseID, TransferredBytes: f.ByteSize, Status: "complete"})
	if err == nil && n != 1 {
		return Error("REMOTE_TRANSFER_UNAVAILABLE")
	}
	return err
}
