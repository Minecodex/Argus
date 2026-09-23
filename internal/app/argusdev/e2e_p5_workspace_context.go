package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) p5WorkspaceContextProbe(ctx context.Context, env *E2EEnvironment, conversation, label string, files []string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	value, err := client.JSON(ctx, "p5-context-"+label, "enterprise", http.MethodPost, "/conversations/"+conversation+"/messages", 202,
		map[string]any{"content": "argus_e2e_workspace_context", "file_ids": files}, enterpriseHeaders(env, p5RequestKey("p5-context", label)))
	if err != nil {
		return nil, err
	}
	run, err := stringField(value, "run", "run_id")
	if err != nil {
		return nil, err
	}
	if err := a.waitRunTerminal(ctx, env, run); err != nil {
		return nil, err
	}
	content, err := a.postgresQuery(ctx, env, "SELECT payload->>'content' FROM conversation_events WHERE run_id='"+run+"' AND event_type='assistant_message' ORDER BY sequence DESC LIMIT 1;")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		return nil, fmt.Errorf("P5 provider did not receive workspace facts: %w", err)
	}
	return result, nil
}

func (a *App) verifyP5DeletedWorkspaceContext(ctx context.Context, env *E2EEnvironment, conversation string, before map[string]any) error {
	oldID, err := stringField(before, "workspace", "id")
	if err != nil {
		return err
	}
	deleted, err := a.p5WorkspaceContextProbe(ctx, env, conversation, "deleted", []string{})
	if err != nil {
		return err
	}
	status, _ := nestedString(deleted, "workspace", "status")
	id, _ := nestedString(deleted, "workspace", "id")
	if status != "deleted" || id != oldID {
		return fmt.Errorf("P5 actual model request did not carry the deleted workspace identity")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	content := []byte("value\n99\n")
	upload, err := client.JSON(ctx, "p5-context-recreate-upload", "enterprise", http.MethodPost, "/conversations/"+conversation+"/workspace/uploads", 201, map[string]any{"name": "business.csv", "byte_size": len(content)}, enterpriseHeaders(env, "p5-context-recreate-upload"))
	if err != nil {
		return err
	}
	uploadID, err := stringField(upload, "id")
	if err != nil {
		return err
	}
	data, _, err := p5FileHTTP(ctx, client, env, http.MethodPut, "/conversations/"+conversation+"/workspace/uploads/"+uploadID+"/content", content, 201, "")
	if err != nil {
		return err
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	fileID, err := stringField(file, "id")
	if err != nil {
		return err
	}
	recreated, err := a.p5WorkspaceContextProbe(ctx, env, conversation, "recreated", []string{fileID})
	if err != nil {
		return err
	}
	newID, err := stringField(recreated, "workspace", "id")
	if err != nil {
		return err
	}
	status, _ = nestedString(recreated, "workspace", "status")
	available, err := numberField(recreated, "available_references")
	if err != nil || newID == oldID || status != "ready" || available < 1 {
		return fmt.Errorf("P5 new workspace/file reference was not distinct from deleted history")
	}
	if _, err := client.JSON(ctx, "p5-context-delete-new", "enterprise", http.MethodDelete, "/conversations/"+conversation+"/workspace", 202, nil, enterpriseHeaders(env, "p5-context-delete-new")); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM workspaces WHERE conversation_id='"+conversation+"' AND status<>'deleted';", "0", 3*time.Minute); err != nil {
		return err
	}
	evidence, _ := json.Marshal(map[string]any{"before": before, "deleted": deleted, "recreated": recreated, "live_workspaces_after_cleanup": 0, "source": "actual provider request"})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-workspace-context.json"), append(evidence, '\n'))
}
