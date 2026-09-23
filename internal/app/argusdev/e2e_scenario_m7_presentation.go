package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Tool-owned presentation is verified against an actual authorized query;
// Collector package discovery has no dependency on a separate UI catalog.
func (a *App) verifyM7OverviewPresentation(ctx context.Context, env *E2EEnvironment, resourceID string) error {
	conversation, err := a.p5Conversation(ctx, env, "telemetry overview")
	if err != nil {
		return err
	}
	run, err := a.p5Run(ctx, env, conversation, "m7-overview", []p5ToolStep{
		{"tool.describe", map[string]any{"category": "metric", "name": "overview"}},
		{"tool.invoke", map[string]any{"category": "metric", "name": "overview", "arguments": map[string]any{"resource_ids": []string{resourceID}, "lookback_seconds": 3600}}},
	}, []string{})
	if err != nil {
		return err
	}
	call, err := a.postgresQuery(ctx, env, "SELECT p.tool_call_id::text FROM tool_presentations p JOIN tool_calls t ON t.id=p.tool_call_id WHERE t.run_id='"+run+"' AND t.status='succeeded' AND p.tool_version='telemetry.overview/v1';")
	if err != nil {
		return err
	}
	call = strings.TrimSpace(call)
	if call == "" {
		return fmt.Errorf("M7 overview query did not publish its Tool-owned presentation")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	view, err := client.JSON(ctx, "m7-overview-presentation", "enterprise", http.MethodGet, "/conversations/"+conversation+"/tool-presentations/"+call, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if view["runtime"] != "argus-template/v1" || view["template_source"] == "" {
		return fmt.Errorf("M7 overview template is unavailable")
	}
	return nil
}
