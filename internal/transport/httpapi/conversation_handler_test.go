package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestAssistantStreamRetainsActualToolRequestArguments(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"content": nil, "tool_calls": []any{map[string]any{"id": "call", "name": "tool.invoke", "arguments": `{"tool_id":"telemetry.promql.query","arguments":{"query":"cpu"}}`}}})
	envelope, ok := toAgentStreamEnvelope(db.ConversationEvent{ID: uuid.New(), ConversationID: uuid.New(), EventType: "assistant_message", Payload: payload, OccurredAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}, 1)
	if !ok {
		t.Fatal("assistant event omitted")
	}
	agent := envelope["data"].(map[string]any)
	public := agent["payload"].(map[string]any)
	calls, ok := public["tool_calls"].([]any)
	if !ok || len(calls) != 1 || calls[0].(map[string]any)["id"] != "call" {
		t.Fatal("actual request lost", public)
	}
}

func TestToConversationRunIncludesTerminalReason(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	value := db.Run{
		ID: uuid.New(), ConversationID: uuid.New(), EnterpriseID: uuid.New(), ModelID: uuid.New(),
		ModelRevision: 1, Locale: "en-US", Status: "failed", AuthorizationVersion: 1, Version: 2,
		StopReason: pgtype.Text{String: "quota_exceeded", Valid: true},
		ErrorCode:  pgtype.Text{String: "MODEL_QUOTA_EXCEEDED", Valid: true},
		CreatedAt:  now, UpdatedAt: now,
	}

	result := toConversationRun(value)
	if result.StopReason == nil || *result.StopReason != value.StopReason.String {
		t.Fatalf("stop_reason = %v", result.StopReason)
	}
	if result.ErrorCode == nil || *result.ErrorCode != value.ErrorCode.String {
		t.Fatalf("error_code = %v", result.ErrorCode)
	}
}
