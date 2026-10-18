package remoteexecution

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestPlanRegistryPairRemainsHostOnly(t *testing.T) {
	registry := mcp.NewRegistry()
	service := &Service{}
	if err := service.RegisterTools(registry); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidatePairs(); err != nil {
		t.Fatal(err)
	}
	for _, tool := range registry.ModelCatalog() {
		if tool.ID == "workflow.remote_plan.commit" {
			t.Fatal("host commit leaked into model catalog")
		}
	}
	if _, err := registry.Call(t.Context(), mcp.Call{ToolID: "workflow.remote_plan.commit", Caller: "model"}); !errors.Is(err, mcp.ErrCommitCaller) {
		t.Fatal("model could reach commit", err)
	}
}

func TestModelCannotSupplyApprovalOrFrozenContextFields(t *testing.T) {
	contribution, err := (&Service{}).BuildTools(t.Context(), toolruntime.Principal{Permissions: []string{"remote_execution.execute"}})
	if err != nil || len(contribution.Tools) != 1 {
		t.Fatal(err)
	}
	called := false
	contribution.Tools[0].Invoke = func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
		called = true
		return toolruntime.Result{}, nil
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{}, contribution.Tools)
	if err != nil {
		t.Fatal(err)
	}
	for _, injected := range []string{"approved", "action_ref", "session_generation", "identity", "known_directory", "interpreter_version", "previous_request_id", "previous_sequence"} {
		t.Run(injected, func(t *testing.T) {
			step := map[string]any{"description": "inspect", "target": map[string]any{"resource_type": "host", "resource_id": uuid.NewString()}, "interpreter": "bash", "script": "id"}
			args := map[string]any{"title": "Inspect", "description": "Preview only", "steps": []any{step}}
			if injected == "approved" || injected == "action_ref" {
				args[injected] = true
			} else {
				step[injected] = uuid.NewString()
			}
			_, err := set.Invoke(t.Context(), "connect", toolruntime.Invocation{Arguments: args})
			var invalid toolruntime.Error
			if !errors.As(err, &invalid) || invalid.Kind != "TOOL_INPUT_INVALID" || called {
				t.Fatalf("model supplied privileged field %s: %v", injected, err)
			}
		})
	}
}
