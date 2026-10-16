package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPostgresCategoryDiscoveryPersistsCompleteListAndToolSequence(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if err := postgres.RunMigrations(t.Context(), address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	f := newRecoveryFixture(t)
	registry := mcp.NewRegistry()
	for i := range 80 {
		id := fmt.Sprintf("host.inspect_%04d", i)
		if err := registry.Register(mcp.Metadata{ID: id, Visibility: mcp.Visible, Risk: "read", Required: []string{"host.read"}, InputVersion: "1", OutputVersion: "1", MaxResultBytes: 1024,
			Discovery:   mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: "Inspect owned fixture", Description: "Read an isolated fixture to verify the complete discovery protocol.", Keywords: []string{"fixture"}, ResultDescription: "Returns a fixture value.", Preconditions: []string{"Owned test scope."}, Examples: []mcp.DiscoveryExample{{Request: "Read the fixture", Guidance: "Use the empty parameter object."}}},
			InputSchema: map[string]any{"type": "object", "additionalProperties": false}, Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
				return mcp.Result{Structured: map[string]any{"value": "owned fixture"}}, nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	gateway, err := toolgateway.New(registry, "category-sequence-test")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	p := toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c, AuthorizationVersion: 1, Permissions: []string{"host.read"}}
	loop := Loop{Store: f.store}
	ctx, cancel := withTaskLease(t.Context(), f.store, f.task("agent", "category discovery"))
	defer cancel()
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	projectionHash := sha256.Sum256([]byte("fixture"))
	for i, request := range []struct{ name, args string }{
		{"tool.search", `{"category":"host"}`},
		{"tool.describe", `{"category":"host","name":"inspect_0079"}`},
		{"tool.invoke", `{"category":"host","name":"inspect_0079","arguments":{}}`},
	} {
		step, err := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, Sequence: int32(i + 1), StepType: "model_call", Status: "running"})
		if err != nil {
			t.Fatal(err)
		}
		modelCall, err := f.store.Queries.CreateModelCall(ctx, db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: projectionHash[:], InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
		if err != nil {
			t.Fatal(err)
		}
		f.exec("UPDATE runs SET current_step_id=$2 WHERE id=$1", f.r, step.ID)
		results, err := loop.executeToolCalls(ctx, f.run(), step, modelCall, set, p, true, "", "owned fixture", []*pendingToolCall{{ID: uuid.NewString(), Name: request.name, Arguments: request.args}})
		if err != nil {
			t.Fatal(err)
		}
		var projection map[string]any
		if err = json.Unmarshal([]byte(results[0].message), &projection); err != nil {
			t.Fatal(err)
		}
		summary := projection["summary"].(map[string]any)
		if i == 0 && (projection["partial"] != false || len(summary["items"].([]any)) != 80 || summary["complete"] != true) {
			t.Fatal("persisted/model projection lost a listed tool")
		}
		if i == 1 && summary["name"] != "inspect_0079" {
			t.Fatal("description is not the selected last tool")
		}
		if i == 2 && summary["value"] != "owned fixture" {
			t.Fatal("selected tool was not executed")
		}
	}
	if f.count("SELECT count(*) FROM tool_calls WHERE run_id=$1 AND status='succeeded'", f.r) != 3 {
		t.Fatal("search/describe/invoke facts not persisted")
	}
}
