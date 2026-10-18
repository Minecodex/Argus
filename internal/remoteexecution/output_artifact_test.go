package remoteexecution

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type outputSink struct {
	artifacts []OutputArtifact
	contents  []string
}

func (*outputSink) ValidateRemoteInputs(context.Context, toolruntime.Principal, []File) error {
	return nil
}
func (*outputSink) FreezeRemoteInputs(context.Context, toolruntime.Principal, uuid.UUID, uuid.UUID) ([]File, error) {
	return nil, nil
}
func (s *outputSink) ImportRemoteOutput(_ context.Context, _ toolruntime.Principal, a OutputArtifact, reader io.Reader) (db.WorkspaceFile, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	s.artifacts = append(s.artifacts, a)
	s.contents = append(s.contents, string(data))
	return db.WorkspaceFile{Path: "results/nginx/status (2).txt"}, nil
}

type sealedOutputs struct{ data map[string]string }

func (s *sealedOutputs) PutFile(_ context.Context, key string, reader io.Reader, size int64, _ string) error {
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(body)) != size {
		return Error("REMOTE_OUTPUT_INTEGRITY")
	}
	s.data[key] = string(body)
	return nil
}
func (s *sealedOutputs) ReadFile(_ context.Context, key string, _, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.data[key])), nil
}

func TestRemoteOutputSealsBytesAndReturnsActualBusinessPath(t *testing.T) {
	f := newFixture(t)
	view, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("inspect nginx"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := f.store.Queries.GetRemoteRequest(t.Context(), db.GetRemoteRequestParams{ID: view.ID, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	sink := &outputSink{}
	f.service.Files = sink
	f.service.Objects = &sealedOutputs{data: map[string]string{}}
	first, err := f.service.storeOutput(t.Context(), request, view.Spec.Steps[0], "nginx/status.txt", []byte("actual evidence"))
	if err != nil || first.Path != "results/nginx/status (2).txt" {
		t.Fatalf("returned path ignored importer collision: %+v %v", first, err)
	}
	again, err := f.service.storeOutput(t.Context(), request, view.Spec.Steps[0], "nginx/status.txt", []byte("actual evidence"))
	if err != nil || again.ID != first.ID || len(sink.artifacts) != 2 || sink.artifacts[0] != sink.artifacts[1] {
		t.Fatalf("replay changed immutable provenance: %+v %v", sink.artifacts, err)
	}
	artifact := sink.artifacts[0]
	if artifact.RequestID != view.ID || artifact.StepID != view.Spec.Steps[0].ID || artifact.SessionID != view.Spec.Steps[0].SessionID || artifact.RunID != f.r || artifact.Target.ResourceID != f.h || artifact.SourcePath != "nginx/status.txt" || artifact.RequestTitle != view.Spec.Title || sink.contents[0] != "actual evidence" {
		t.Fatalf("provenance missing: %+v", artifact)
	}
	if _, err := f.service.storeOutput(t.Context(), request, view.Spec.Steps[0], "nginx/status.txt", []byte("changed bytes")); err == nil {
		t.Fatal("same remote artifact accepted changed bytes")
	}
	if len(sink.artifacts) != 2 {
		t.Fatal("changed immutable artifact reached workspace importer")
	}
}
