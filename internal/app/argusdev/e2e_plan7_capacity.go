package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Each model scenario uses a fresh chat while previous compute sessions may
// still be warm. Wait for the normal lifecycle to free a slot; do not increase
// the quota, shorten idle policies or delete the persistent evidence files.
func (a *App) plan7WorkspaceCapacity(ctx context.Context, env *E2EEnvironment, label string) (failure error) {
	enterprise, err := uuid.Parse(env.State.Values["enterprise_id"])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	observations := []map[string]any{}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "scenario": label, "ready": failure == nil, "observations": observations}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan7-workspace-capacity-"+p5RequestKey("scenario", label)+".json"), raw))
	}()
	query := fmt.Sprintf(`SELECT json_build_object('limit',q.max_concurrent_sessions,'active',(SELECT count(*) FROM sandbox_sessions s WHERE s.enterprise_id=q.enterprise_id AND s.status IN ('creating','running','terminating','unknown'))) FROM sandbox_quotas q WHERE q.enterprise_id='%s'`, enterprise)
	for {
		raw, err := a.postgresQuery(ctx, env, query)
		if err != nil {
			return err
		}
		var state struct {
			Limit  int `json:"limit"`
			Active int `json:"active"`
		}
		if json.Unmarshal([]byte(raw), &state) != nil || state.Limit < 1 {
			return fmt.Errorf("Plan7 compute quota is not configured")
		}
		observations = append(observations, map[string]any{"at": time.Now().UTC(), "limit": state.Limit, "active": state.Active})
		if state.Active < state.Limit {
			return nil
		}
		if err := waitContext(ctx, 5*time.Second); err != nil {
			return fmt.Errorf("Plan7 compute capacity did not become available: %w", err)
		}
	}
}

func (a *App) plan7AssertWorkspaceTools(ctx context.Context, env *E2EEnvironment, run string) error {
	if uuid.Validate(run) != nil {
		return fmt.Errorf("invalid capability Run")
	}
	raw, err := a.postgresQuery(ctx, env, `SELECT json_build_object('run_id',id,'workspace_status',tool_snapshot->>'sandbox_status','tools',jsonb_path_query_array(tool_snapshot,'$.tools[*].model.name')) FROM runs WHERE id='`+run+`'`)
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan7-run-tools-"+run+".json"), []byte(strings.TrimSpace(raw))); err != nil {
		return err
	}
	var snapshot struct {
		Workspace string   `json:"workspace_status"`
		Tools     []string `json:"tools"`
	}
	if json.Unmarshal([]byte(raw), &snapshot) != nil || snapshot.Workspace != "ready" {
		return fmt.Errorf("Workspace tools unavailable at Run start: %s", snapshot.Workspace)
	}
	for _, name := range []string{"read", "write", "edit", "bash"} {
		if !slices.Contains(snapshot.Tools, name) {
			return fmt.Errorf("Run is missing builtin Workspace tool %s", name)
		}
	}
	return nil
}
