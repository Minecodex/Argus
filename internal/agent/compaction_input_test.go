package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
)

func TestCompactedActiveRunRetainsOriginalUserInstructionsAndFileState(t *testing.T) {
	f := newRecoveryFixture(t)
	f.exec(`UPDATE runs SET tool_snapshot='{"version":"argus.model_tool_set/v1","tools":[]}' WHERE id=$1`, f.r)
	scope, err := presentation.Scope(t.Context(), f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	previous := uuid.New()
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'succeeded')", previous, f.c, f.e, f.u, f.m)
	appendEvent := func(run uuid.UUID, kind string, payload map[string]any) int64 {
		event, err := conversation.AppendEvent(t.Context(), f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: run, Valid: true}, Type: kind, ActorType: "user", ActorID: f.u.String(), Payload: payload, Classification: "internal"})
		if err != nil {
			t.Fatal(err)
		}
		return event.Sequence
	}
	appendEvent(previous, "user_message", map[string]any{"content": "An earlier completed task must not become a fresh instruction."})
	goal := "只修复测试服务 svc-A；禁止修改其他资源；执行前必须确认。"
	feedback := "先备份配置，原目标与所有限制继续有效。"
	appendEvent(f.r, "user_message", map[string]any{"content": goal, "files": []map[string]any{{"id": uuid.NewString()}}})
	appendPair := func(id, result string) int64 {
		appendEvent(f.r, "assistant_message", map[string]any{"authorization_scope": scope, "tool_calls": []modelprovider.ToolCall{{ID: id, Name: "read", Arguments: `{}`}}})
		return appendEvent(f.r, "tool_call_result", map[string]any{"authorization_scope": scope, "tool_call_id": id, "projection": map[string]any{"data": result}})
	}
	appendPair("call-a", strings.Repeat("collected evidence\n", 1500))
	appendEvent(f.r, "user_message", map[string]any{"content": feedback})
	through := appendPair("call-b", "backup proposal was reviewed; execution facts stay authoritative in PostgreSQL")
	appendPair("call-c", "the retained native tool pair")
	checkpoint, _ := json.Marshal(scopedCheckpoint{AuthorizationScope: scope, Execution: json.RawMessage(`{}`)})
	f.exec("INSERT INTO context_snapshots(id,enterprise_id,conversation_id,run_id,revision,source_from_sequence,source_through_sequence,first_kept_sequence,typed_checkpoint,narrative_summary,compaction_model_id,compaction_model_revision,prompt_version,estimated_tokens_before,actual_tokens_after,source_hash,snapshot_hash,status) VALUES($1,$2,$3,$4,1,1,$5,$5::bigint+1,$6,'A lossy historical summary cannot replace user constraints.',$7,1,'argus.compaction/v2',15000,20,decode(repeat('00',32),'hex'),decode(repeat('01',32),'hex'),'active')", uuid.New(), f.e, f.c, f.r, through, checkpoint, f.m)
	messages, _, _, _, err := (Loop{Store: f.store}).messages(t.Context(), f.run())
	if err != nil {
		t.Fatal(err)
	}
	goalIndex, feedbackIndex, goals, feedbacks := -1, -1, 0, 0
	for i, message := range messages {
		if strings.Contains(message.Content, "An earlier completed task must not become") {
			t.Fatal("a prior Run's user message was reactivated")
		}
		if message.Role == "user" && strings.HasPrefix(message.Content, goal) {
			goals++
			goalIndex = i
			if !strings.Contains(message.Content, "WORKSPACE_FILE_UNAVAILABLE") {
				t.Fatal("unavailable attachment was revived")
			}
		}
		if message.Role == "user" && message.Content == feedback {
			feedbacks++
			feedbackIndex = i
		}
	}
	if goals != 1 || feedbacks != 1 || goalIndex >= feedbackIndex {
		t.Fatalf("original instructions and revision not retained in order: %d/%d at %d/%d", goals, feedbacks, goalIndex, feedbackIndex)
	}
	for _, protocol := range []modelprovider.Protocol{modelprovider.ProtocolChatCompletions, modelprovider.ProtocolResponses} {
		if _, err := (modelprovider.Provider{Protocol: protocol}).RequestBytes(modelprovider.Request{Model: "fixture", Messages: messages, MaxTokens: 1024}); err != nil {
			t.Fatalf("retained native pair is invalid for %s: %v", protocol, err)
		}
	}
}
