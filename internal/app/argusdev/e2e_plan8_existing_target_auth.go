package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/kakj-go/Argus/internal/monitoring"
)

func (a *App) plan8ExistingTargetAuthorization(ctx context.Context, env *E2EEnvironment) error {
	var services struct {
		Passed      bool                    `json:"passed"`
		Collections []monitoring.Collection `json:"collections"`
	}
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-deployed-services.json"))
	if err != nil || json.Unmarshal(raw, &services) != nil || !services.Passed {
		return errors.New("existing target authorization requires actual database collection evidence")
	}
	var extra struct {
		Passed    bool `json:"passed"`
		Resources []struct {
			Collection monitoring.Collection `json:"collection"`
		} `json:"resources"`
	}
	raw, err = os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-deployed-extra-services.json"))
	if err != nil || json.Unmarshal(raw, &extra) != nil || !extra.Passed {
		return errors.New("existing target authorization requires actual Redis/Nginx evidence")
	}
	rows := services.Collections
	for _, record := range extra.Resources {
		rows = append(rows, record.Collection)
	}
	metrics := map[string]string{"mysql": "mysql_uptime", "sqlserver": "sqlserver_user_connection_count", "redis": "redis_uptime", "nginx": "nginx_connections_accepted"}
	proofs := []map[string]any{}
	for _, row := range rows {
		metric := metrics[row.Integration]
		if metric == "" {
			return errors.New("unexpected existing target authorization combination")
		}
		current, e := a.plan8ServiceCollection(ctx, env, row.ID)
		if e != nil {
			return e
		}
		proof, e := a.plan8VerifyTargetAuthorization(ctx, env, current, metric)
		if e != nil {
			return e
		}
		proof["integration"] = row.Integration
		proofs = append(proofs, proof)
	}
	body, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "actual_api_account_mfa_target_grants_historical_metrics_and_per_target_revocation", "resources": proofs}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-existing-target-authorization.json"), body)
}
