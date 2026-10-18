package remoteexecution

import (
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type recursiveWorkspace struct {
	service *Service
	request uuid.UUID
	denied  bool
	checks  int
}

func (w *recursiveWorkspace) FreezeRemoteInputs(_ context.Context, _ toolruntime.Principal, _, _ uuid.UUID) ([]File, error) {
	return []File{{ID: uuid.New(), WorkspaceID: uuid.New(), Path: "input.txt", Hash: hashBytes(nil), ObjectKey: "sealed-fixture"}}, nil
}
func (w *recursiveWorkspace) ValidateRemoteInputs(ctx context.Context, p toolruntime.Principal, _ []File) error {
	w.checks++
	if w.checks > 5 {
		return Error("TEST_RECURSION_LIMIT")
	}
	if w.request != uuid.Nil {
		if _, err := w.service.AuthorizeSource(ctx, p, "remote:"+w.request.String()); err != nil {
			return err
		}
	}
	if w.denied {
		return Error("REMOTE_RESOURCE_FORBIDDEN")
	}
	return nil
}
func (*recursiveWorkspace) ImportRemoteOutput(context.Context, toolruntime.Principal, OutputArtifact, io.Reader) (db.WorkspaceFile, error) {
	return db.WorkspaceFile{}, Error("TEST_UNEXPECTED_IMPORT")
}

func TestRemoteInputSourceCyclesRemainBoundedAndRevocationIsRechecked(t *testing.T) {
	f := newFixture(t)
	workspace := &recursiveWorkspace{service: f.service}
	f.service.Files = workspace
	call := f.call()
	view, err := f.service.Prepare(t.Context(), call, "connect", f.input("inspect"))
	if err != nil {
		t.Fatal(err)
	}
	workspace.request = view.ID
	workspace.checks = 0
	if _, err = f.service.Get(t.Context(), call.Principal, view.ID); err != nil || workspace.checks != 1 {
		t.Fatalf("input provenance cycle did not terminate: checks=%d err=%v", workspace.checks, err)
	}
	workspace.denied = true
	if _, err = f.service.Get(t.Context(), call.Principal, view.ID); !errorsIsCode(err, "REMOTE_RESOURCE_FORBIDDEN") {
		t.Fatalf("previous source validation bypassed current revocation: %v", err)
	}
}
