package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestTruncatedAssistantKeepsReportedUsageAndDoesNotClaimTaskSuccess(t *testing.T) {
	for _, example := range []struct{ reason, text, status string }{
		{"length", "", "failed"},
		{"length", "Partial recovery analysis", "failed"},
		{"incomplete", "Partial response", "failed"},
		{"content_filter", "", "failed"},
		{"stop", "Completed result", "succeeded"},
	} {
		t.Run(example.reason+example.text, func(t *testing.T) {
			f := newRecoveryFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": example.text}, "finish_reason": example.reason}}})
				fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":64}}\n\ndata: [DONE]\n\n", raw)
			}))
			defer server.Close()
			var usage modelprovider.TokenUsage
			var reason, text string
			provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: server.URL, Client: server.Client()}
			if err := provider.Stream(t.Context(), modelprovider.Request{Model: "fixture", MaxTokens: 64}, func(event modelprovider.Event) error {
				usage.Observe(event)
				if event.Type == "completed" {
					reason = event.StopReason
				}
				if event.Type == "text_delta" {
					text += event.Text
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !usage.Complete() {
				t.Fatal("provider usage missing")
			}
			step, err := f.store.Queries.CreateRunStep(t.Context(), db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := withTaskLease(context.WithoutCancel(t.Context()), f.store, f.task("agent", "completion status"))
			defer cancel()
			if err = (Loop{Store: f.store}).finishAssistantTurn(ctx, f.run(), step, text, "", usage, reason); err != nil {
				t.Fatal(err)
			}
			run := f.run()
			if run.Status != example.status {
				t.Fatalf("truncated generation became task success: %s", run.Status)
			}
			if example.status == "failed" && (run.ErrorCode.String != "MODEL_OUTPUT_INCOMPLETE" || run.StopReason.String != "model_output_incomplete") {
				t.Fatal("incomplete task lost actionable failure")
			}
			if f.count("SELECT count(*) FROM run_steps WHERE id=$1 AND status=$2", step.ID, example.status) != 1 {
				t.Fatal("step state contradicts task state")
			}
			if f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='model_usage' AND payload->>'input_tokens'='120' AND payload->>'output_tokens'='64' AND payload->>'usage_complete'='true'", f.r) != 1 {
				t.Fatal("truncation lost actual provider usage")
			}
			if f.count("SELECT count(*) FROM tool_calls WHERE run_id=$1", f.r) != 0 {
				t.Fatal("incomplete task dispatched a tool")
			}
		})
	}
}
