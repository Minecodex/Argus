package agent

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestUIActivityDoesNotEnterModelProjectionOrCompaction(t *testing.T) {
	activity := &toolruntime.Activity{Schema: "argus.tool_activity/v1", Kind: "file_change", Path: "/workspace/file", Change: &toolruntime.FileChange{Operation: "create", Lines: []toolruntime.DiffLine{{Type: "add", Content: "ui-only-snapshot"}}}}
	result := toolruntime.Result{Data: map[string]any{"path": "/workspace/file"}, Activity: activity}
	full, _ := json.Marshal(result.Data)
	projection, _, err := encodeToolResultProjection("ref", "call", full, result)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err = json.Unmarshal(projection, &data); err != nil {
		t.Fatal(err)
	}
	if data["activity"] != nil {
		t.Fatal("UI evidence reached model projection")
	}
	raw, _ := json.Marshal(map[string]any{"authorization_scope": "scope", "projection": data, "activity": activity})
	for _, scope := range []string{"scope", "revoked"} {
		payload, err := contextEventPayload("tool_call_result", raw, scope)
		if err != nil || payload["activity"] != nil {
			t.Fatal("UI evidence reached compaction", err)
		}
	}
}

func TestToolActivityLifecycleIsDurableAndExternalEvidenceCannotSpoofNative(t *testing.T) {
	for _, source := range []string{"argus", "sandbox", "external_mcp"} {
		t.Run(source, func(t *testing.T) {
			f := newRecoveryFixture(t)
			ctx, cancel := withTaskLease(t.Context(), f.store, f.task("agent", "activity lifecycle"))
			defer cancel()
			invoked := 0
			set, err := toolruntime.NewSet(toolruntime.Snapshot{}, []toolruntime.Tool{{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "read", Schema: map[string]any{"type": "object"}}, Source: source, ReadOnly: true, Version: "fixture"}, Invoke: func(ctx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
				invoked++
				if source == "external_mcp" {
					if err := call.BeforeDispatch(ctx); err != nil {
						return toolruntime.Result{}, err
					}
				}
				return toolruntime.Result{Data: map[string]any{"content": "actual fixture output"}, Activity: &toolruntime.Activity{Schema: "argus.tool_activity/v1", Kind: "file_read", Path: "/workspace/actual"}}, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			step, model, _ := argumentModelResponse(t, f, ctx, 1, `{}`)
			scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
			if err != nil {
				t.Fatal(err)
			}
			loop := Loop{Store: f.store}
			principal := toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c}
			if _, err := loop.executeToolCalls(ctx, f.run(), step, model, set, principal, false, "Checking", scope, []*pendingToolCall{{ID: "one", Name: "read", Arguments: `{}`}}); err != nil {
				t.Fatal(err)
			}
			events, err := f.store.Queries.ListConversationEvents(ctx, db.ListConversationEventsParams{EnterpriseID: f.e, ConversationID: f.c, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			started, finished := int64(0), int64(0)
			var callID string
			for _, event := range events {
				var payload map[string]any
				if json.Unmarshal(event.Payload, &payload) != nil {
					continue
				}
				if event.EventType == "tool_call_started" {
					started = event.Sequence
					callID = payload["tool_call_id"].(string)
					if payload["source"] != source {
						t.Fatal("source lost")
					}
				}
				if event.EventType == "tool_call_result" {
					finished = event.Sequence
					if payload["tool_call_id"] != callID {
						t.Fatal("start/result identity changed")
					}
					if (payload["activity"] == nil) != (source == "external_mcp") {
						t.Fatal("provider evidence trust boundary broken")
					}
				}
			}
			if started == 0 || finished <= started {
				t.Fatal("missing or misordered dispatch event")
			}
			if _, err := loop.executePersistedTool(ctx, f.run(), set, principal, false, db.ToolCall{ID: uuid.MustParse(callID), Source: source}); err != nil {
				t.Fatal(err)
			}
			if n := f.count(`SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='tool_call_started'`, f.r); n != 1 || invoked != 1 {
				t.Fatal("duplicate dispatch")
			}
			if source == "sandbox" {
				service := conversation.Service{Store: f.store}
				current, err := service.ListEvents(ctx, f.e, f.u, f.c, 0, 50)
				if err != nil {
					t.Fatal(err)
				}
				hasActivity := func(events []db.ConversationEvent) bool {
					for _, event := range events {
						var p map[string]any
						_ = json.Unmarshal(event.Payload, &p)
						if p["activity"] != nil {
							return true
						}
					}
					return false
				}
				if !hasActivity(current) {
					t.Fatal("authorized evidence was removed")
				}
				f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.u)
				redacted, err := service.ListEvents(ctx, f.e, f.u, f.c, 0, 50)
				if err != nil || hasActivity(redacted) {
					t.Fatal("revoked file snapshots remained readable", err)
				}
			}
		})
	}
}
