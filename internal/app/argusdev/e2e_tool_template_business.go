package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
)

func templateSQLJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return "'" + strings.ReplaceAll(string(raw), "'", "''") + "'::jsonb"
}

// Policy configuration uses the real preview/confirmation API. Historical
// alert facts are explicitly controlled, typed test data in the owned tenant;
// this acceptance verifies real tools/storage/reads/rendering, not evaluation.
func (a *App) templateBusinessFacts(ctx context.Context, env *E2EEnvironment) (string, string, error) {
	enterprise, actor, cluster := env.State.Values["enterprise_id"], env.State.Values["admin_user_id"], env.State.Values["m3_cluster_id"]
	for _, id := range []string{enterprise, actor, cluster} {
		if uuid.Validate(id) != nil {
			return "", "", fmt.Errorf("missing owned template fixture identity")
		}
	}
	sa, role := uuid.NewString(), uuid.NewString()
	sql := fmt.Sprintf(`INSERT INTO service_accounts(id,enterprise_id,name) VALUES('%s','%s','Template evaluator');INSERT INTO roles(id,enterprise_id,name) VALUES('%s','%s','Template reader');INSERT INTO role_permissions(role_id,permission_id) VALUES('%s','kubernetes.read');INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES(gen_random_uuid(),'%s','service_account','%s','%s');INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES(gen_random_uuid(),'%s','service_account','%s','kubernetes_cluster','%s');`, sa, enterprise, role, enterprise, role, enterprise, sa, role, enterprise, sa, cluster)
	if _, err := a.postgresQuery(ctx, env, sql); err != nil {
		return "", "", err
	}
	if _, err := a.plan6Config(ctx, env, "template-settings", map[string]any{"kind": "settings", "operation": "create", "name": "Template evaluator", "spec": map[string]any{"evaluator_id": sa}}); err != nil {
		return "", "", err
	}
	resource := uuid.MustParse(cluster)
	rule := uuid.New()
	name := "Template real policy"
	spec := alerting.PolicySpec{Enabled: false, Resources: alerting.ResourceSelector{Mode: "explicit", Types: []string{"kubernetes_cluster"}, IDs: []uuid.UUID{resource}, SourceIDs: []uuid.UUID{}, Matchers: []alerting.Matcher{}}, Rules: []alerting.Rule{{ID: rule, Name: "Template gauge rule", Expression: "argus_m7_e2e_gauge_planv2", QueryMode: "numeric", Operator: ">", Threshold: 85, RecoveryThreshold: 75, Severity: "critical", Unit: "%"}, {ID: uuid.New(), Name: "Template memory member", Expression: "argus_m7_e2e_gauge_planv2 + 1", QueryMode: "numeric", Operator: ">", Threshold: 3072, RecoveryThreshold: 2048, Severity: "warning", Unit: "MiB"}}, Mode: "every", MatchBy: []string{"argus_resource_id", "argus_source_id"}, IntervalSeconds: 30, NoDataBehavior: "report", ErrorBehavior: "keep", ChannelIDs: []uuid.UUID{}, RouteIDs: []uuid.UUID{}, Labels: map[string]string{"fixture": "template"}}
	created, err := a.plan6Config(ctx, env, "template-policy", map[string]any{"kind": "policy", "operation": "create", "name": name, "spec": spec})
	if err != nil {
		return "", "", err
	}
	policy, err := stringField(created, "id")
	if err != nil {
		return "", "", err
	}
	frozen, _ := json.Marshal(spec)
	spec.Rules[0].Threshold = 95
	spec.Mode = "any"
	if _, err = a.plan6Config(ctx, env, "template-policy-update", map[string]any{"kind": "policy", "operation": "update", "id": policy, "expected_version": created["version"], "name": name, "spec": spec}); err != nil {
		return "", "", err
	}
	instance, history, event, channel, delivery := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	value := 91.
	now := time.Now().UTC()
	state := alerting.InstanceState{Name: "Template historical trigger", PolicyName: name, RuleID: rule, Metric: "argus_m7_e2e_gauge_planv2", Value: &value, Threshold: 85, RecoveryThreshold: 75, Unit: "%", PolicySnapshot: frozen, NotificationState: "failed", NoiseReasons: []string{"silence: template maintenance", "inhibition: template upstream"}, LastSlot: now}
	state.RuleID = uuid.Nil
	state.LastSuccessfulAt = &now
	state.Members = []alerting.MemberValue{{RuleID: rule, Name: "Template gauge rule", Expression: spec.Rules[0].Expression, Value: 91, Threshold: 85, RecoveryThreshold: 75, Severity: "critical", Unit: "%", Firing: true}, {RuleID: spec.Rules[1].ID, Name: "Template memory member", Expression: spec.Rules[1].Expression, Value: 4096.75, Threshold: 3072, RecoveryThreshold: 2048, Severity: "warning", Unit: "MiB", Firing: true}}
	message := alerting.Message{Subject: "Template failed notification", Text: "Controlled template alert 91% > 85%", EventType: "firing"}
	sql = fmt.Sprintf(`BEGIN;
INSERT INTO alert_instances(id,enterprise_id,policy_id,policy_version,generation,fingerprint,resource_ids,source_ids,state,lifecycle,health,severity,started_at,last_evaluated_at) VALUES('%s','%s','%s',1,1,'template-active',ARRAY['%s']::uuid[],'{}',%s,'firing','normal','critical',now()-interval '2 minutes',now());
INSERT INTO alert_instances(id,enterprise_id,policy_id,policy_version,generation,fingerprint,resource_ids,source_ids,state,lifecycle,health,severity,started_at,resolved_at,last_evaluated_at) VALUES('%s','%s','%s',1,1,'template-history',ARRAY['%s']::uuid[],'{}',%s,'resolved','normal','critical',now()-interval '3 minutes',now()-interval '1 minute',now());
INSERT INTO alert_events(id,enterprise_id,instance_id,policy_id,event_type,resource_ids,payload,origin) VALUES('%s','%s','%s','%s','firing',ARRAY['%s']::uuid[],'{"fixture":"typed template history"}','admin_ui');
INSERT INTO alert_configurations(id,enterprise_id,kind,name,spec,created_by) VALUES('%s','%s','channel','Template disabled channel','{"enabled":false,"provider":"smtp"}','%s');
INSERT INTO alert_deliveries(id,enterprise_id,event_id,instance_id,channel_id,channel_version,route_version,dedup_key,group_key,resource_ids,payload,status,attempt,last_error) VALUES('%s','%s','%s','%s','%s',1,1,'template-failed','template',ARRAY['%s']::uuid[],%s,'failed',2,'Controlled SMTP failure');
COMMIT;`, instance, enterprise, policy, cluster, templateSQLJSON(state), history, enterprise, policy, cluster, templateSQLJSON(state), event, enterprise, instance, policy, cluster, channel, enterprise, actor, delivery, enterprise, event, instance, channel, cluster, templateSQLJSON(message))
	if _, err = a.postgresQuery(ctx, env, sql); err != nil {
		return "", "", err
	}
	anyID, anyValue := uuid.NewString(), 97.25
	anyState := state
	anyState.Name, anyState.Value, anyState.Threshold = "Template any-rule trigger", &anyValue, 95
	anyState.PolicySnapshot, _ = json.Marshal(spec)
	anyState.Members = append([]alerting.MemberValue{}, state.Members...)
	anyState.Members[0].Value, anyState.Members[0].Threshold = anyValue, 95
	anyState.Members[1].Value, anyState.Members[1].Firing, anyState.Members[1].Recovering = 1024.75, false, true
	sql = fmt.Sprintf(`INSERT INTO alert_instances(id,enterprise_id,policy_id,policy_version,generation,fingerprint,resource_ids,source_ids,state,lifecycle,health,severity,started_at,last_evaluated_at) VALUES('%s','%s','%s',2,1,'template-any',ARRAY['%s']::uuid[],'{}',%s,'firing','normal','critical',now()-interval '1 minute',now());`, anyID, enterprise, policy, cluster, templateSQLJSON(anyState))
	if _, err = a.postgresQuery(ctx, env, sql); err != nil {
		return "", "", err
	}
	env.State.Values["template_any_instance_id"] = anyID
	if err = a.templateHealthFacts(ctx, env, policy, cluster, state); err != nil {
		return "", "", err
	}
	if err = a.templateHealthKindFacts(ctx, env, policy, cluster, frozen); err != nil {
		return "", "", err
	}
	proof, _ := json.MarshalIndent(map[string]any{"kind": "controlled-business-facts", "policy_created_and_updated_through_confirmation_api": true, "alert_facts": "typed controlled fixtures in owned tenant, not evaluator output", "actual_model_evaluated": false, "policy_id": policy, "instance_id": instance, "historical_instance_id": history, "any_instance_id": anyID, "snapshot_mode": "every", "current_mode": "any", "historical_policy_version": 1, "current_policy_version": 2, "snapshot_threshold": 85, "current_threshold": 95}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "tool-template-business-facts.json"), proof); err != nil {
		return "", "", err
	}
	return policy, instance, nil
}
