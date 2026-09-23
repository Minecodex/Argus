package argusdev

import (
	"context"
	"path/filepath"
	"strings"
)

// Only operational state is exported. Credentials, tool inputs, result bodies,
// private previews and callback trust material are deliberately excluded.
func (a *App) collectScenarioState(ctx context.Context, env *E2EEnvironment) {
	if env.State == nil || !env.installed {
		return
	}
	queries := map[string]string{
		"connection-tests": `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,status,error_code,result->>'architecture' AS architecture,result->>'callback_verified' AS callback_verified,result->>'callback_control_path' AS callback_control_path,created_at,updated_at FROM connection_tests ORDER BY created_at DESC LIMIT 100) t`,
		"runs":             `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,conversation_id,status,stop_reason,error_code,created_at,updated_at FROM runs ORDER BY created_at DESC LIMIT 100) t`,
		"tool-calls":       `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,run_id,tool_id,source,status,error_code,created_at FROM tool_calls ORDER BY created_at DESC LIMIT 100) t`,
		"workspaces":       `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,conversation_id,status,capacity_bytes,fence_token,lease_owner,lease_until,last_used_at,active_pod_name,active_sandbox_id,created_at,updated_at FROM workspaces ORDER BY created_at DESC LIMIT 100) t`,
	}
	for name, query := range queries {
		value, err := a.postgresQuery(ctx, env, query)
		if err == nil {
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "state-"+name+".json"), []byte(strings.TrimSpace(value)+"\n"))
		}
	}
}
