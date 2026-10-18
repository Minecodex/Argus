package remoteexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type transferObjects struct {
	data           []byte
	reads, maxRead int
}
type sizedReader struct {
	*bytes.Reader
	owner *transferObjects
}

func (r *sizedReader) Read(b []byte) (int, error) {
	r.owner.maxRead = max(r.owner.maxRead, len(b))
	return r.Reader.Read(b)
}
func (o *transferObjects) ReadFile(context.Context, string, int64, int64) (io.ReadCloser, error) {
	o.reads++
	return io.NopCloser(&sizedReader{Reader: bytes.NewReader(o.data), owner: o}), nil
}
func (*transferObjects) PutFile(context.Context, string, io.Reader, int64, string) error {
	return errors.New("unexpected output")
}

type inputTransport struct {
	data                     []byte
	writes, finishes, failAt int
	incomplete               bool
}

func (t *inputTransport) Call(_ context.Context, _ db.RemoteExecutionIdentity, kind string, payload proto.Message, _ string, _ time.Duration, _ func(context.Context, *db.Queries, string) error) (json.RawMessage, error) {
	file, ok := payload.(*connectorv1.RemoteExecutionFile)
	if !ok || kind != "remote_execution_file" {
		return nil, errors.New("unexpected command")
	}
	if file.Operation == "write_input" {
		t.writes++
		if t.failAt > 0 && t.writes == t.failAt {
			return nil, errors.New("transfer interrupted")
		}
		end := int(file.Offset) + len(file.Data)
		if end > len(t.data) {
			t.data = append(t.data, make([]byte, end-len(t.data))...)
		}
		copy(t.data[int(file.Offset):], file.Data)
		return protojson.Marshal(&connectorv1.RemoteExecutionFileResult{Offset: uint64(end)})
	}
	if file.Operation == "finish_input" {
		t.finishes++
		return protojson.Marshal(&connectorv1.RemoteExecutionFileResult{Complete: !t.incomplete})
	}
	return nil, errors.New("unexpected file operation")
}

func TestFrozenInputTransfersAreBoundedDurableAndNotReapplied(t *testing.T) {
	for _, scenario := range []string{"success", "interruption", "corrupt_source", "incomplete_receipt"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			view, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("approved"))
			if err != nil {
				t.Fatal(err)
			}
			f.approve(view)
			req, err := f.store.Queries.GetRemoteRequest(t.Context(), db.GetRemoteRequestParams{ID: view.ID, EnterpriseID: f.e})
			if err != nil {
				t.Fatal(err)
			}
			step := view.Spec.Steps[0]
			identity, err := f.store.Queries.GetRemoteIdentityByID(t.Context(), db.GetRemoteIdentityByIDParams{ID: step.Identity.ID, EnterpriseID: f.e})
			if err != nil {
				t.Fatal(err)
			}
			original := bytes.Repeat([]byte("f"), ChunkBytes+19)
			objects := &transferObjects{data: append([]byte(nil), original...)}
			transport := &inputTransport{}
			f.service.Objects = objects
			f.service.Transport = transport
			file, err := f.store.Queries.CreateRemoteFile(t.Context(), db.CreateRemoteFileParams{ID: uuid.New(), EnterpriseID: f.e, RequestID: view.ID, Direction: "input", Path: "input.txt", ByteSize: int64(len(original)), ContentHash: hashBytes(original), ObjectKey: "fixture-frozen", Status: "sealed"})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "interruption" {
				transport.failAt = 2
			}
			if scenario == "corrupt_source" {
				objects.data[0] = 'x'
			}
			if scenario == "incomplete_receipt" {
				transport.incomplete = true
			}
			err = f.service.uploadFrozenInput(t.Context(), req, step, identity, file)
			if scenario != "success" {
				if err == nil {
					t.Fatal("transfer failure reported success")
				}
				var state string
				if e := f.store.Pool.QueryRow(t.Context(), "SELECT status FROM remote_execution_files WHERE request_id=$1 AND session_id=$2 AND direction='input'", view.ID, step.SessionID).Scan(&state); e != nil || state == "complete" {
					t.Fatal("failed transfer published complete receipt", e)
				}
				if scenario != "interruption" {
					return
				}
				transport.failAt = 0
				ctx := context.WithValue(t.Context(), transferAttemptKey{}, int32(2))
				if err = f.service.uploadFrozenInput(ctx, req, step, identity, file); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(transport.data, original) || objects.maxRead > ChunkBytes {
				t.Fatal("streamed input differs or exceeds its buffer limit")
			}
			writes, reads := transport.writes, objects.reads
			transport.data = []byte("approved prior step changed the temporary input")
			if err = f.service.uploadFrozenInput(context.WithValue(t.Context(), transferAttemptKey{}, int32(3)), req, step, identity, file); err != nil {
				t.Fatal(err)
			}
			if transport.writes != writes || objects.reads != reads || string(transport.data) != "approved prior step changed the temporary input" {
				t.Fatal("worker retry silently re-uploaded an already delivered input")
			}
			var size int64
			if err = f.store.Pool.QueryRow(t.Context(), "SELECT transferred_bytes FROM remote_execution_files WHERE request_id=$1 AND session_id=$2 AND status='complete'", view.ID, step.SessionID).Scan(&size); err != nil || size != int64(len(original)) {
				t.Fatal("per-target transfer receipt missing", err)
			}
		})
	}
}
