package remoteexecution

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/proto"
)

type failOpenTransport struct{ *controlledTransport }

func (f failOpenTransport) Call(ctx context.Context, identity db.RemoteExecutionIdentity, kind string, payload proto.Message, key string, timeout time.Duration, queued func(context.Context, *db.Queries, string) error) (json.RawMessage, error) {
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok && control.Operation == "open" {
		return nil, Error("REMOTE_CONTEXT_LOST")
	}
	return f.controlledTransport.Call(ctx, identity, kind, payload, key, timeout, queued)
}

func TestModelFactsDistinguishUnsentAndUnknownWithoutInventingExitCode(t *testing.T) {
	for _, tc := range []struct {
		name            string
		unknown, unsent bool
		dispatch        string
		hasExit         bool
	}{
		{"known completion", false, false, "sent", true},
		{"missing receipt", true, false, "sent", false},
		{"lost during preparation", false, true, "not_sent", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if tc.unknown {
				f.transport.unknown = 1
			}
			if tc.unsent {
				f.service.Transport = failOpenTransport{f.transport}
			}
			v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("effect"))
			if err != nil {
				t.Fatal(err)
			}
			f.approve(v)
			if err = f.run(v); err != nil {
				t.Fatal(err)
			}
			run, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: f.r, EnterpriseID: f.e})
			if err != nil {
				t.Fatal(err)
			}
			facts, err := f.service.ModelFacts(t.Context(), run)
			if err != nil {
				t.Fatal(err)
			}
			executions := facts["executions"].([]map[string]any)
			step := executions[0]["steps"].([]map[string]any)[0]
			_, hasExit := step["exit_code"]
			if step["dispatch_state"] != tc.dispatch || hasExit != tc.hasExit {
				t.Fatalf("misleading execution facts: %+v", step)
			}
			if tc.unsent && f.transport.batches.Load() != 0 {
				t.Fatal("preparation loss dispatched the command")
			}
		})
	}
}
