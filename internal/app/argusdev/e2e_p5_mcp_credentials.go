package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

func (a *App) verifyP5MCPCredentialBoundary(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	for _, auth := range []string{"bearer", "basic"} {
		for _, wire := range []string{"json", "sse"} {
			label := "credential-" + auth + "-" + wire
			value := "p5-token"
			if auth == "basic" {
				value = `{"username":"p5","password":"p5-pass"}`
			}
			body := map[string]any{"name": "P5 unsafe " + label, "endpoint": "https://argus-replay-model." + env.ReplayNamespace() + ".svc/mcp?auth=" + auth + "&mode=" + wire + "&reflect=catalog", "auth_type": auth, "credential_value": value, "member_ids": []string{env.State.Values["admin_user_id"]}}
			rejected, err := client.JSON(ctx, "p5-unsafe-catalog-"+label, "enterprise", http.MethodPost, "/enterprise/mcp-connections", 422, body, enterpriseHeaders(env, "p5-unsafe-catalog-"+label))
			if err != nil {
				return err
			}
			if rejected["code"] != "MCP_RESPONSE_CREDENTIAL_EXPOSED" {
				return fmt.Errorf("P5 unsafe catalog error was not controlled")
			}
			if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM mcp_connections WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' AND name='P5 unsafe "+label+"';", "0", time.Minute); err != nil {
				return err
			}
			connection, err := a.p5MCPConnection(ctx, env, label, "auth="+auth+"&mode="+wire+"&reflect=result", auth)
			if err != nil {
				return err
			}
			id, err := stringField(connection, "id")
			if err != nil {
				return err
			}
			convo, err := a.p5Conversation(ctx, env, label)
			if err != nil {
				return err
			}
			if err = a.p5SelectConnections(ctx, env, convo, []string{id}); err != nil {
				return err
			}
			run, err := a.p5StartRun(ctx, env, convo, label, []p5ToolStep{{"mcp:write_counter", map[string]any{}}}, []string{})
			if err != nil {
				return err
			}
			if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runs WHERE id='"+run+"' AND status='failed' AND stop_reason='result_unknown';", "1", time.Minute); err != nil {
				return err
			}
			if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id WHERE t.run_id='"+run+"' AND t.status='result_unknown' AND r.projection->'summary'->>'cause'='MCP_RESPONSE_CREDENTIAL_EXPOSED';", "1", time.Minute); err != nil {
				return err
			}
			// Inspect only this test's stored surfaces, including the complete
			// artifact (not just the redacted model projection).
			query := `SELECT count(*) FROM (
 SELECT convert_from(content,'UTF8') AS body FROM artifacts WHERE run_id='` + run + `'
 UNION ALL SELECT payload::text FROM conversation_events WHERE run_id='` + run + `'
 UNION ALL SELECT tool_snapshot::text FROM runs WHERE id='` + run + `'
 UNION ALL SELECT tools::text FROM mcp_tool_snapshots WHERE connection_id='` + id + `'
) stored WHERE body LIKE '%p5-token%' OR body LIKE '%p5-pass%' OR body LIKE '%cDU6cDUtcGFzcw==%';`
			if err = a.waitPostgresValue(ctx, env, query, "0", time.Minute); err != nil {
				return fmt.Errorf("P5 managed credential escaped a persisted surface: %w", err)
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"authentication_modes": 2, "wire_modes": 2, "unsafe_catalogs_rejected": 4, "unsafe_calls_result_unknown": 4, "unsafe_payloads_persisted": 0})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-mcp-credential-boundary.json"), append(data, '\n'))
}
