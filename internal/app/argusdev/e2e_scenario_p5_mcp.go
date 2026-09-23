package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (a *App) p5MCPConnection(ctx context.Context, env *E2EEnvironment, name, query, auth string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"name": "P5 " + name, "endpoint": "https://argus-replay-model." + env.ReplayNamespace() + ".svc/mcp?" + query, "auth_type": auth, "member_ids": []string{env.State.Values["admin_user_id"]}}
	if auth == "bearer" {
		body["credential_value"] = "p5-token"
	}
	if auth == "basic" {
		body["credential_value"] = `{"username":"p5","password":"p5-pass"}`
	}
	value, err := client.JSON(ctx, "p5-mcp-create-"+name, "enterprise", http.MethodPost, "/enterprise/mcp-connections", 201, body, enterpriseHeaders(env, "p5-mcp-create-"+name))
	if err != nil {
		return nil, err
	}
	id, err := stringField(value, "id")
	if err != nil {
		return nil, err
	}
	tested, err := client.JSON(ctx, "p5-mcp-test-"+name, "enterprise", http.MethodPost, "/enterprise/mcp-connections/"+id+"/test", 200, nil, enterpriseHeaders(env, "p5-mcp-test-"+name))
	if err != nil {
		return nil, err
	}
	if tested["health_status"] != "healthy" || tested["tool_count"] != float64(2) {
		return nil, fmt.Errorf("P5 MCP initialization/pagination failed: %v", tested["health_status"])
	}
	if _, exists := tested["credential_value"]; exists {
		return nil, fmt.Errorf("P5 MCP response leaked credential")
	}
	return tested, nil
}
func (a *App) p5SelectConnections(ctx context.Context, env *E2EEnvironment, conversation string, ids []string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	current, err := client.JSON(ctx, "p5-current-selection", "enterprise", http.MethodGet, "/conversations/"+conversation, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	_, err = client.JSON(ctx, "p5-save-selection", "enterprise", http.MethodPut, "/conversations/"+conversation, 200, map[string]any{"expected_version": current["version"], "selected_mcp_connection_ids": ids}, enterpriseHeaders(env, ""))
	return err
}

func (a *App) verifyP5MCP(ctx context.Context, env *E2EEnvironment, mode string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	for _, transport := range []string{"json", "sse"} {
		title := mode + "-" + transport
		conversation, err := a.p5Conversation(ctx, env, title)
		if err != nil {
			return err
		}
		before, err := client.JSON(ctx, "p5-preflight-without-mcp-"+title, "enterprise", http.MethodPost, "/conversations/"+conversation+"/preflight", 200, map[string]any{"content": "Read current state", "file_ids": []string{}}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		want := float64(3)
		if mode == "agent-sandbox" {
			want = 7
		}
		if before["tool_count"] != want {
			return fmt.Errorf("P5 %s initial tool count = %v, want %v", title, before["tool_count"], want)
		}
		auth := "bearer"
		if transport == "sse" {
			auth = "basic"
		}
		connection, err := a.p5MCPConnection(ctx, env, title, "mode="+transport+"&auth="+auth, auth)
		if err != nil {
			return err
		}
		id, _ := stringField(connection, "id")
		if err := a.p5SelectConnections(ctx, env, conversation, []string{id}); err != nil {
			return err
		}
		after, err := client.JSON(ctx, "p5-preflight-with-mcp-"+title, "enterprise", http.MethodPost, "/conversations/"+conversation+"/preflight", 200, map[string]any{"content": "Read current state", "file_ids": []string{}}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		if after["tool_count"] != want+2 {
			return fmt.Errorf("P5 selected MCP tools were not directly injected")
		}
		run, err := a.p5Run(ctx, env, conversation, title, []p5ToolStep{{"mcp:write_counter", map[string]any{}}}, []string{})
		if err != nil {
			return err
		}
		counts, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND source='external_mcp' AND status='succeeded';")
		if err != nil {
			return err
		}
		if strings.TrimSpace(counts) != "1" {
			return fmt.Errorf("P5 Remote MCP did not execute directly")
		}
		count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_presentations p JOIN tool_calls t ON t.id=p.tool_call_id WHERE t.run_id='"+run+"';")
		if err != nil {
			return err
		}
		if strings.TrimSpace(count) != "0" {
			return fmt.Errorf("P5 customer MCP incorrectly created a template")
		}
		if transport == "json" {
			beforeWrites, err := a.p5Counter(ctx, env, run)
			if err != nil {
				return err
			}
			unknown, _ := a.p5Run(ctx, env, conversation, title+"-unknown", []p5ToolStep{{"mcp:write_counter", map[string]any{"drop_response": true}}, {"tool.search", map[string]any{"category": "host"}}}, []string{})
			if unknown == "" {
				return fmt.Errorf("P5 unknown-result run was not accepted")
			}
			count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+unknown+"' AND status='result_unknown';")
			if err != nil {
				return err
			}
			if strings.TrimSpace(count) != "1" {
				return fmt.Errorf("P5 interrupted MCP write was not recorded as result_unknown")
			}
			count, err = a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+unknown+"';")
			if err != nil {
				return err
			}
			if strings.TrimSpace(count) != "1" {
				return fmt.Errorf("P5 Run advanced after unknown MCP write")
			}
			read, err := a.p5Run(ctx, env, conversation, title+"-verify-unknown", []p5ToolStep{{"mcp:read_counter", map[string]any{}}}, []string{})
			if err != nil {
				return err
			}
			afterWrites, err := a.p5Counter(ctx, env, read)
			if err != nil {
				return err
			}
			if afterWrites != beforeWrites+1 {
				return fmt.Errorf("P5 MCP write was replayed after unknown outcome")
			}
		}
		current, err := client.JSON(ctx, "p5-mcp-current-"+title, "enterprise", http.MethodGet, "/enterprise/mcp-connections/"+id, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		if _, err := client.JSON(ctx, "p5-mcp-revoke-"+title, "enterprise", http.MethodPut, "/enterprise/mcp-connections/"+id+"/members", 200, map[string]any{"member_ids": []string{}, "expected_version": current["version"]}, enterpriseHeaders(env, "")); err != nil {
			return err
		}
		if _, err := client.JSON(ctx, "p5-mcp-revoked-preflight-"+title, "enterprise", http.MethodPost, "/conversations/"+conversation+"/preflight", 403, map[string]any{"content": "Do not execute revoked tools"}, enterpriseHeaders(env, "")); err != nil {
			return err
		}
	}
	large, err := a.p5MCPConnection(ctx, env, mode+"-large", "large=1", "none")
	if err != nil {
		return err
	}
	largeID, _ := stringField(large, "id")
	conversation, err := a.p5Conversation(ctx, env, mode+"-capacity")
	if err != nil {
		return err
	}
	env.State.Values["p5_capacity_conversation_id"] = conversation
	if err := a.p5SelectConnections(ctx, env, conversation, []string{largeID}); err != nil {
		return err
	}
	beforeTasks, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM runtime_tasks WHERE queue='agent' AND enterprise_id='"+env.State.Values["enterprise_id"]+"';")
	if err != nil {
		return err
	}
	preflight, err := client.JSON(ctx, "p5-capacity-preflight-"+mode, "enterprise", http.MethodPost, "/conversations/"+conversation+"/preflight", 422, map[string]any{"content": "Keep this input intact", "file_ids": []string{}}, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if preflight["code"] != "MODEL_TOOL_CAPACITY_EXCEEDED" {
		return fmt.Errorf("P5 preflight did not report capacity exhaustion")
	}
	failure, err := client.JSON(ctx, "p5-capacity-reject-"+mode, "enterprise", http.MethodPost, "/conversations/"+conversation+"/messages", 422, map[string]any{"content": "Keep this input intact", "file_ids": []string{}}, enterpriseHeaders(env, "p5-capacity-reject-"+mode))
	if err != nil {
		return err
	}
	if failure["code"] != "MODEL_TOOL_CAPACITY_EXCEEDED" {
		return fmt.Errorf("P5 capacity rejection code = %v", failure["code"])
	}
	count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM runs WHERE conversation_id='"+conversation+"';")
	if err != nil {
		return err
	}
	if strings.TrimSpace(count) != "0" {
		return fmt.Errorf("P5 capacity rejection created a Run")
	}
	afterTasks, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM runtime_tasks WHERE queue='agent' AND enterprise_id='"+env.State.Values["enterprise_id"]+"';")
	if err != nil {
		return err
	}
	if strings.TrimSpace(afterTasks) != strings.TrimSpace(beforeTasks) {
		return fmt.Errorf("P5 capacity rejection created an execution task")
	}

	return nil
}
func (a *App) p5Counter(ctx context.Context, env *E2EEnvironment, run string) (int64, error) {
	value, err := a.postgresQuery(ctx, env, "SELECT projection->'summary'->'data'->>'writes' FROM tool_results r JOIN tool_calls t ON t.id=r.tool_call_id WHERE t.run_id='"+run+"' AND t.source='external_mcp' ORDER BY t.created_at DESC LIMIT 1;")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
}
