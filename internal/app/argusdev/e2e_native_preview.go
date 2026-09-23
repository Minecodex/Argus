package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Exercise the model gateway against the same real business fixture as HTTP.
// Cancel this independent Preview before the existing deterministic commit test.
func (a *App) verifyNativePreview(ctx context.Context, env *E2EEnvironment, key, category, name string, input map[string]any) error {
	conversation, err := a.p5Conversation(ctx, env, "native-preview-"+key)
	if err != nil {
		return err
	}
	run, err := a.p5StartRun(ctx, env, conversation, "native-preview-"+key, []p5ToolStep{
		{"tool.describe", map[string]any{"category": category, "name": name}},
		{"tool.invoke", map[string]any{"category": category, "name": name, "arguments": input}},
	}, []string{})
	if err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, run, "waiting_input", 90*time.Second); err != nil {
		return err
	}
	value, err := a.postgresQuery(ctx, env, "SELECT a.action_ref FROM pending_actions a WHERE a.run_id='"+run+"' AND a.status='awaiting_confirmation' AND EXISTS (SELECT 1 FROM tool_presentations p JOIN tool_calls t ON p.tool_call_id=t.id WHERE t.run_id=a.run_id);")
	if err != nil {
		return err
	}
	action := strings.TrimSpace(value)
	if action == "" || strings.ContainsAny(action, "\r\n") {
		return fmt.Errorf("native %s Preview lacks its authoritative action, template or originating Run", name)
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "native-preview-cancel-"+key, "enterprise", http.MethodPost, "/enterprise/pending-actions/"+action+"/cancel", http.StatusOK, nil, enterpriseHeaders(env, p5RequestKey("p5-preview-cancel", key))); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runs WHERE id='"+run+"' AND status='cancelled' AND stop_reason='pending_action_cancelled';", "1", 10*time.Second); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM conversation_events WHERE run_id='"+run+"' AND event_type='run_state_changed' AND payload->>'status'='cancelled';", "1", time.Second); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stdout, "Native %s.%s Preview/template/Run binding passed\n", category, name)
	return nil
}
