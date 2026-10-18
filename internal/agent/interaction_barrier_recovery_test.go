package agent

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/interaction"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestCommittedPlanBarrierRecoversWithoutModelOrLaterToolDispatch(t *testing.T) {
	f, s, p, original, probe := interactionFixture(t)
	task := f.task("agent", "crash after plan commit")
	ctx, cancel := withTaskLease(t.Context(), f.store, task)
	var tools []toolruntime.Tool
	for _, definition := range original.Snapshot.Definitions {
		tool, _ := original.Lookup(definition.Model.Name)
		if tool.Model.Name == "tool.invoke" {
			invoke := tool.Invoke
			tool.Invoke = func(callCtx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
				result, err := invoke(callCtx, call)
				if result.Control != nil && result.Control.Kind == "plan" {
					cancel()
				}
				return result, err
			}
		}
		tools = append(tools, tool)
	}
	set, err := toolruntime.NewSet(original.Snapshot, tools)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"category": "workflow", "name": "plan.propose", "arguments": interaction.Plan{Title: "Durable plan", Markdown: "# Durable plan\n\nInspect the fixture."}})
	step, modelCall, pending := argumentModelResponse(t, f, ctx, 1, string(args))
	pending.Name = "tool.invoke"
	scope, err := presentation.Scope(t.Context(), f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Loop{Store: f.store}).executeToolCalls(ctx, f.run(), step, modelCall, set, p, false, "", scope, []*pendingToolCall{pending, {ID: "must-not-run", Name: "after_question", Arguments: `{}`, Index: 1}})
	if err == nil {
		t.Fatal("fault did not interrupt ToolResult persistence")
	}
	cancel()
	if *probe != 0 || f.count("SELECT count(*) FROM conversation_interactions WHERE run_id=$1 AND status='pending'", f.r) != 1 {
		t.Fatal("plan commit or no-dispatch invariant lost")
	}
	if f.count("SELECT count(*) FROM tool_results WHERE enterprise_id=$1", f.e) != 0 {
		t.Fatal("fault did not leave the intended recovery gap")
	}
	f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", task.ID)
	recoveryTask := f.task("agent", "recover plan barrier")
	recoveryCtx, stop := withTaskLease(t.Context(), f.store, recoveryTask)
	defer stop()
	paused, err := (Loop{Store: f.store}).reconcileInteractions(recoveryCtx, f.run())
	if err != nil || !paused {
		t.Fatalf("plan barrier recovery failed: %v", err)
	}
	if *probe != 0 || f.count("SELECT count(*) FROM tool_results WHERE enterprise_id=$1", f.e) != 2 {
		t.Fatal("recovery dispatched a later tool or left unpaired outcomes")
	}
	items, err := s.List(t.Context(), p)
	if err != nil || len(items) != 1 || items[0].Status != "pending" {
		t.Fatalf("recovered plan preview unavailable: %v", err)
	}
	if f.count("SELECT count(*) FROM model_calls WHERE run_id=$1", f.r) != 1 {
		t.Fatal("recovery required another model call")
	}
	// Cross-conversation IDs never reveal the recovered plan.
	other := p
	other.ConversationID = uuid.New()
	if _, err = s.Get(t.Context(), other, items[0].ID); err == nil {
		t.Fatal("cross-conversation plan access succeeded")
	}
}
