package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) verifyPlan6DynamicAdmission(ctx context.Context, env *E2EEnvironment) error {
	marker := env.Options.RunID
	policy := map[string]any{"enabled": true, "resources": map[string]any{"mode": "dynamic", "types": []string{"host"}, "ids": []string{}, "source_ids": []string{}, "matchers": []map[string]string{{"key": "plan6_auto", "operator": "=", "value": marker}}}, "rules": []map[string]any{{"name": "CPU", "expression": "argus_plan6_cpu_percent", "query_mode": "numeric", "operator": ">", "threshold": 85, "recovery_threshold": 75, "severity": "critical", "unit": "%"}}, "mode": "independent", "match_by": []string{"argus_resource_id", "argus_source_id"}, "interval_seconds": 10, "for_seconds": 0, "recovery_seconds": 0, "no_data_behavior": "keep", "error_behavior": "keep", "channel_ids": []string{}, "route_ids": []string{}, "labels": map[string]string{}}
	created, err := a.plan6Config(ctx, env, "dynamic-admission", map[string]any{"kind": "policy", "operation": "create", "name": "Plan6 dynamic admission", "spec": policy})
	if err != nil {
		return err
	}
	id, _ := stringField(created, "id")
	if uuid.Validate(id) != nil {
		return fmt.Errorf("dynamic policy identity missing")
	}
	host := uuid.NewString()
	enterprise, sa, actor := env.State.Values["enterprise_id"], env.State.Values["p6_evaluator_id"], env.State.Values["admin_user_id"]
	for _, value := range []string{enterprise, sa, actor} {
		if uuid.Validate(value) != nil {
			return fmt.Errorf("dynamic fixture identity invalid")
		}
	}
	labels, _ := json.Marshal(map[string]string{"plan6_auto": marker})
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf(`INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels,labels_hash) VALUES('%s','%s','Plan6 automatically admitted','192.0.2.6',22,'linux','managed_host','direct','production','%s',decode(repeat('00',32),'hex'));`, host, enterprise, string(labels))); err != nil {
		return err
	}
	// The new resource alone must not expand the evaluator's explicit authority.
	deadline := time.Now().Add(time.Minute)
	latest := ""
	for {
		latest, err = a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT json_build_object('resources',scope->'resources','health',health,'slot',slot) FROM alert_evaluations WHERE policy_id='%s' ORDER BY slot DESC LIMIT 1;`, id))
		if err != nil {
			return err
		}
		if strings.TrimSpace(latest) != "" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dynamic empty scope was not recorded")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
	if strings.Contains(latest, host) {
		return fmt.Errorf("new host expanded evaluator authority without grant")
	}
	before := json.RawMessage(strings.TrimSpace(latest))
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES(gen_random_uuid(),'%s','service_account','%s','host','%s'),(gen_random_uuid(),'%s','user','%s','host','%s'); UPDATE service_accounts SET authorization_version=authorization_version+1 WHERE enterprise_id='%s' AND id='%s';`, enterprise, sa, host, enterprise, actor, host, enterprise, sa)); err != nil {
		return err
	}
	deadline = time.Now().Add(time.Minute)
	for {
		latest, err = a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT json_build_object('resources',scope->'resources','health',health,'slot',slot,'authorization_version',authorization_version) FROM alert_evaluations WHERE policy_id='%s' ORDER BY slot DESC LIMIT 1;`, id))
		if err != nil {
			return err
		}
		if strings.Contains(latest, host) {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("new authorized resource did not join dynamic policy")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
	var after map[string]any
	if json.Unmarshal([]byte(latest), &after) != nil || after["health"] != "no_data" {
		return fmt.Errorf("host without collector was wrongly declared healthy")
	}
	raw, _ := json.MarshalIndent(map[string]any{"passed": true, "no_implicit_grant": true, "dynamic_authorized_admission": true, "missing_source_is_no_data": true, "new_resource_id": host, "policy_id": id, "before_grant": before, "after_grant": after}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan6-dynamic-admission.json"), raw)
}
