package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/interaction"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"sync"
	"testing"
)

type interactionProbe struct{ count *int }

func (p interactionProbe) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	return toolruntime.Contribution{Tools: []toolruntime.Tool{{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "after_question", Schema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}}, Source: "argus", ReadOnly: true}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
		*p.count++
		return toolruntime.Result{Data: map[string]any{"observed": true}}, nil
	}}}}, nil
}

func interactionFixture(t *testing.T) (*recoveryFixture, interaction.Service, toolruntime.Principal, *toolruntime.Set, *int) {
	t.Helper()
	f := newRecoveryFixture(t)
	role := uuid.New()
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Interaction operator')", role, f.e)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.e, f.u, role)
	c := conversation.Service{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{7}, 32)}}
	s := interaction.Service{Store: f.store, Conversations: c, Idempotency: c.Idempotency, Remote: &remoteexecution.Service{Store: f.store}}
	registry := mcp.NewRegistry()
	if err := s.RegisterTools(registry); err != nil {
		t.Fatal(err)
	}
	gateway, err := toolgateway.New(registry, "interactions-test")
	if err != nil {
		t.Fatal(err)
	}
	count := new(int)
	factory := toolruntime.CompositeFactory{Providers: []toolruntime.Provider{gateway, s, interactionProbe{count}}}
	s.Conversations.Tools = factory
	p, err := s.Conversations.ToolPrincipal(t.Context(), f.e, f.u, f.c)
	if err != nil {
		t.Fatal(err)
	}
	set, err := factory.Build(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return f, s, p, set, count
}
func runInteractionCall(t *testing.T, f *recoveryFixture, set *toolruntime.Set, p toolruntime.Principal, sequence int, name string, args any, tail bool) ([]toolExecutionResult, context.Context, func()) {
	t.Helper()
	task := f.task("agent", "interaction control")
	ctx, cancelLease := withTaskLease(t.Context(), f.store, task)
	cancel := func() {
		cancelLease()
		f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", task.ID)
	}
	raw, _ := json.Marshal(args)
	step, call, pending := argumentModelResponse(t, f, ctx, sequence, string(raw))
	pending.Name = name
	calls := []*pendingToolCall{pending}
	if tail {
		calls = append(calls, &pendingToolCall{ID: "later-independent-call", Name: "after_question", Arguments: `{}`, Index: 1})
	}
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	results, err := (Loop{Store: f.store}).executeToolCalls(ctx, f.run(), step, call, set, p, false, "", scope, calls)
	if err != nil {
		t.Fatal(err)
	}
	return results, ctx, cancel
}
func TestUserInputDurablyPausesAllFourModeCombinations(t *testing.T) {
	for _, mode := range []string{"execute", "plan"} {
		for _, access := range []string{"request_approval", "full_access"} {
			t.Run(mode+"/"+access, func(t *testing.T) {
				f, s, p, set, count := interactionFixture(t)
				f.exec("UPDATE conversations SET mode=$2,access_mode=$3 WHERE id=$1", f.c, mode, access)
				f.exec("UPDATE runs SET mode_snapshot=$2,access_mode_snapshot=$3 WHERE id=$1", f.r, mode, access)
				input := interaction.Questions{Questions: []interaction.Question{{ID: "scope", Header: "Scope", Question: "Which scope?", Options: []interaction.Option{{Label: "Fixture", Description: "Owned target"}, {Label: "Custom", Description: "Describe another scope"}}}}}
				results, ctx, cancel := runInteractionCall(t, f, set, p, 1, "request_user_input", input, true)
				defer cancel()
				if len(results) != 1 || !results[0].paused || *count != 0 || f.run().Status != "waiting_input" {
					t.Fatal("question did not stop subsequent dispatch")
				}
				if f.count("SELECT count(*) FROM tool_results WHERE tool_call_id IN (SELECT id FROM tool_calls WHERE run_id=$1)", f.r) != 0 {
					t.Fatal("pending question produced a fabricated answer")
				}
				requests, err := s.List(t.Context(), p)
				if err != nil || len(requests) != 1 {
					t.Fatalf("durable request missing: %v", err)
				}
				q := requests[0]
				answer := interaction.Response{Action: "answer", Version: q.Version, ContentHash: q.ContentHash, Answers: map[string]interaction.Answer{"scope": {SelectedOption: "Fixture", Text: "Use only the test directory"}}}
				key := uuid.NewString()
				if _, err = s.Respond(t.Context(), p, q.ID, answer, key, "en-US"); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Respond(t.Context(), p, q.ID, answer, key, "en-US"); err != nil {
					t.Fatal(err)
				}
				if f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND payload->>'reason'='interaction_resolved'", f.r) != 1 {
					t.Fatal("duplicate answer resumed more than once")
				}
				recovered, err := (Loop{Store: f.store}).recoverToolCalls(ctx, f.run(), set, p, false)
				if err != nil || !recovered || *count != 0 {
					t.Fatalf("answer did not recover paired tool calls: %v", err)
				}
				if f.count("SELECT count(*) FROM tool_results WHERE tool_call_id IN (SELECT id FROM tool_calls WHERE run_id=$1)", f.r) != 2 {
					t.Fatal("native calls and outcomes are not paired")
				}
				answer.Answers["scope"] = interaction.Answer{Text: "A conflicting answer"}
				if _, err = s.Respond(t.Context(), p, q.ID, answer, uuid.NewString(), "en-US"); err == nil {
					t.Fatal("accepted conflicting duplicate answer")
				}
			})
		}
	}
}
func TestGuidancePlanRevisionsAndAtomicConcurrentImplementation(t *testing.T) {
	f, s, p, set, _ := interactionFixture(t)
	f.exec("UPDATE conversations SET mode='plan',access_mode='full_access' WHERE id=$1", f.c)
	f.exec("UPDATE runs SET mode_snapshot='plan' WHERE id=$1", f.r)
	plan := interaction.Plan{Title: "Repair fixture", Markdown: "# Repair fixture\n\nUse the owned test target."}
	propose := func(sequence int, plan interaction.Plan) interaction.View {
		t.Helper()
		f.exec("UPDATE runs SET status='running' WHERE id=$1", f.r)
		results, _, cancel := runInteractionCall(t, f, set, p, sequence, "tool.invoke", map[string]any{"category": "workflow", "name": "plan.propose", "arguments": plan}, true)
		cancel()
		if len(results) != 1 || !results[0].paused {
			t.Fatalf("plan did not create a host confirmation: %#v", results)
		}
		items, err := s.List(t.Context(), p)
		if err != nil || len(items) != 1 {
			t.Fatalf("latest draft projection: %v %#v", err, items)
		}
		return items[0]
	}
	first := propose(1, plan)
	if _, err := s.Respond(t.Context(), p, first.ID, interaction.Response{Action: "revise", Version: first.Version, ContentHash: first.ContentHash, Feedback: "Verify before modifying"}, uuid.NewString(), "zh-CN"); err != nil {
		t.Fatal(err)
	}
	plan.SupersedesID = &first.ID
	plan.Markdown += "\n\nVerify the target before modifying it."
	second := propose(2, plan)
	if second.Revision != 2 || second.FamilyID != first.FamilyID {
		t.Fatal("revision lost plan identity")
	}
	stale := interaction.Response{Action: "confirm", Version: first.Version, ContentHash: first.ContentHash}
	if _, err := s.Respond(t.Context(), p, first.ID, stale, uuid.NewString(), "zh-CN"); err == nil {
		t.Fatal("superseded draft was executable")
	}
	input := interaction.Response{Action: "confirm", Version: second.Version, ContentHash: second.ContentHash}
	outputs := make(chan interaction.View, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := s.Respond(t.Context(), p, second.ID, input, uuid.NewString(), "zh-CN")
			outputs <- v
			errs <- err
		}()
	}
	wg.Wait()
	close(outputs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id uuid.UUID
	for v := range outputs {
		if v.ImplementationRunID == nil {
			t.Fatal("missing implementation Run")
		}
		if id != uuid.Nil && id != *v.ImplementationRunID {
			t.Fatal("duplicate implementation")
		}
		id = *v.ImplementationRunID
	}
	run, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: id, EnterpriseID: f.e})
	if err != nil || run.ModeSnapshot != "execute" || run.AccessModeSnapshot != "full_access" || run.ApprovedPlanID.UUID != second.ID {
		t.Fatalf("implementation mode or plan snapshot missing: %v", err)
	}
	facts, _, err := conversation.ContextFacts(t.Context(), f.store.Queries, f.e, f.u, f.c)
	if err != nil || facts["approved_plan"] == nil {
		t.Fatal("approved plan is not pinned after resume/compaction")
	}
	if f.count("SELECT count(*) FROM runs WHERE conversation_id=$1 AND status='pending'", f.c) != 1 || f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1 AND event_type='user_message' AND payload->>'content'='是的，执行此计划'", f.c) != 1 {
		t.Fatal("confirmation and dispatch were not atomic/idempotent")
	}
	if f.count("SELECT count(*) FROM user_confirmations WHERE enterprise_id=$1", f.e) != 0 {
		t.Fatal("guidance plan acceptance granted command authorization")
	}
}
func TestUserInputSkipStopAndAuthorizationInvalidation(t *testing.T) {
	for _, operation := range []string{"skip", "stop", "revoke"} {
		t.Run(operation, func(t *testing.T) {
			f, s, p, set, _ := interactionFixture(t)
			input := interaction.Questions{Questions: []interaction.Question{{ID: "context", Header: "Context", Question: "Describe the context", Options: []interaction.Option{}}}}
			_, ctx, cancel := runInteractionCall(t, f, set, p, 1, "request_user_input", input, false)
			defer cancel()
			items, err := s.List(t.Context(), p)
			if err != nil {
				t.Fatal(err)
			}
			q := items[0]
			response := interaction.Response{Action: "skip", Version: q.Version, ContentHash: q.ContentHash}
			switch operation {
			case "skip":
				if _, err = s.Respond(t.Context(), p, q.ID, response, uuid.NewString(), "en-US"); err != nil {
					t.Fatal(err)
				}
				if _, err = (Loop{Store: f.store}).recoverToolCalls(ctx, f.run(), set, p, false); err != nil {
					t.Fatal(err)
				}
				if f.count("SELECT count(*) FROM tool_results WHERE projection::text LIKE '%skipped%' AND enterprise_id=$1", f.e) != 1 {
					t.Fatal("skip invented a default answer")
				}
			case "stop":
				err = f.store.InTx(t.Context(), func(q *db.Queries) error {
					_, e := conversation.CancelRunRecord(t.Context(), q, f.run(), "user_cancelled", "user", f.u.String())
					return e
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.Respond(t.Context(), p, q.ID, response, uuid.NewString(), "en-US"); err == nil {
					t.Fatal("answered a stopped run")
				}
			case "revoke":
				f.exec("UPDATE enterprise_users SET authorization_version=2 WHERE id=$1", f.u)
				p.AuthorizationVersion = 2
				if _, err = s.Get(t.Context(), p, q.ID); err == nil {
					t.Fatal("revoked source remained readable")
				}
				if _, err = s.Respond(t.Context(), p, q.ID, response, uuid.NewString(), "en-US"); err == nil {
					t.Fatal("revoked question resumed")
				}
			}
		})
	}
}
