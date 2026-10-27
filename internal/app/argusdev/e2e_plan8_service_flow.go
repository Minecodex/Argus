package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan8ServiceCollection(ctx context.Context, env *E2EEnvironment, id uuid.UUID) (monitoring.Collection, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return monitoring.Collection{}, err
	}
	value, err := client.JSON(ctx, "plan8-service-read-"+id.String(), "enterprise", http.MethodGet, "/monitoring/collections/"+id.String(), 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return monitoring.Collection{}, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return monitoring.Collection{}, err
	}
	var row monitoring.Collection
	err = json.Unmarshal(body, &row)
	return row, err
}

func (a *App) plan8ServiceCommit(ctx context.Context, env *E2EEnvironment, label string, input map[string]any) (uuid.UUID, error) {
	if input["kind"] == "collection" || input["kind"] == "endpoint" {
		if err := a.plan8WaitCollectorPaths(ctx, env); err != nil {
			return uuid.Nil, err
		}
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return uuid.Nil, err
	}
	preview, err := client.JSON(ctx, "plan8-service-preview-"+label, "enterprise", http.MethodPost, "/monitoring/commands/preview", 201, input, enterpriseHeaders(env, "plan8-service-preview-"+uuid.NewString()))
	if err != nil {
		a.plan8ServiceReadinessEvidence(env, label)
		return uuid.Nil, err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return uuid.Nil, err
	}
	result, err := a.confirmPendingAction(ctx, env, "plan8-service-confirm-"+label, ref)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := stringField(result, "resource_ref", "resource_id")
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(id)
}

func (a *App) plan8ServiceReadinessEvidence(env *E2EEnvironment, label string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Operational identities/state only; never include payloads, credentials,
	// endpoint URLs, environment contents or private previews.
	query := `SELECT coalesce(json_agg(t),'[]') FROM (SELECT c.id,c.role,c.status,c.version,c.desired_revision,c.effective_revision,c.resource_type,c.resource_id,h.status AS host_status,h.control_path,x.status AS connector_status,x.connection_epoch,r.kind AS route_kind,r.transport,r.status AS route_status,b.status AS bastion_status,b.relay_status,bx.status AS bastion_connector_status FROM collector_instances c LEFT JOIN hosts h ON h.id=c.resource_id AND h.enterprise_id=c.enterprise_id AND c.resource_type='host' LEFT JOIN connectors x ON x.id=h.connector_id AND x.enterprise_id=h.enterprise_id LEFT JOIN telemetry_routes r ON r.collector_id=c.id AND r.enterprise_id=c.enterprise_id LEFT JOIN bastion_scopes b ON b.id=h.bastion_scope_id AND b.enterprise_id=h.enterprise_id LEFT JOIN connectors bx ON bx.id=b.active_connector_id AND bx.enterprise_id=b.enterprise_id ORDER BY c.created_at) t`
	state, err := a.postgresQuery(ctx, env, query)
	proof := map[string]any{"stage": label, "state_query_succeeded": err == nil}
	if err == nil {
		var rows any
		if json.Unmarshal([]byte(state), &rows) == nil {
			proof["collectors"] = rows
		}
	}
	config, err := env.Kube.Client.CoreV1().ConfigMaps(env.SystemNS).Get(ctx, "argus-runtime-config", metav1.GetOptions{})
	if err == nil && config.Labels["argus.io/release-id"] == env.ReleaseID {
		proof["telemetry_catalog_setting"] = config.Data["ARGUS_TELEMETRY_TOOL_CATALOG_ENABLED"]
	}
	body, _ := json.MarshalIndent(proof, "", "  ")
	_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-service-"+label+"-readiness.json"), body)
}

func (a *App) plan8ServiceEffective(ctx context.Context, env *E2EEnvironment, id, collector uuid.UUID, enabled bool, after *time.Time) (monitoring.Collection, error) {
	return a.plan8WaitServiceEffective(ctx, env, id, collector, enabled, after, false)
}

func (a *App) plan8WaitServiceEffective(ctx context.Context, env *E2EEnvironment, id, collector uuid.UUID, enabled bool, after *time.Time, waitUnknown bool) (monitoring.Collection, error) {
	deadline := time.Now().Add(3 * time.Minute)
	for {
		row, err := a.plan8ServiceCollection(ctx, env, id)
		if err != nil {
			return row, err
		}
		if row.ApplyStatus == "failed" || row.ApplyStatus == "unknown" && !waitUnknown {
			a.plan8ServiceFailureEvidence(env, row)
			return row, fmt.Errorf("service configuration failed: %s", row.ErrorCode)
		}
		effective := row.ApplyStatus == "effective" && row.EffectiveRevision == row.DesiredRevision && row.DesiredEnabled == enabled
		if enabled {
			effective = effective && row.EffectiveCollectorID != nil && *row.EffectiveCollectorID == collector && row.LastSampleAt != nil && (after == nil || row.LastSampleAt.After(*after))
		}
		if effective {
			return row, nil
		}
		if time.Now().After(deadline) {
			a.plan8ServiceFailureEvidence(env, row)
			return row, fmt.Errorf("service %s did not converge: apply=%s desired=%d effective=%d health=%s freshness=%s code=%s", id, row.ApplyStatus, row.DesiredRevision, row.EffectiveRevision, row.Health, row.Freshness, row.ErrorCode)
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return row, err
		}
	}
}

func (a *App) plan8ServiceUpdate(ctx context.Context, env *E2EEnvironment, label string, row monitoring.Collection, collector uuid.UUID, enabled bool) (monitoring.Collection, error) {
	input := plan8ServiceUpdateBody(row, collector, enabled)
	if collector == row.CollectorID && enabled != row.DesiredEnabled {
		input = plan8ServiceEnabledBody(row, enabled)
	}
	if _, err := a.plan8ServiceCommit(ctx, env, label, input); err != nil {
		return row, err
	}
	return a.plan8ServiceEffective(ctx, env, row.ID, collector, enabled, row.LastSampleAt)
}

func (a *App) plan8ServiceMetric(ctx context.Context, env *E2EEnvironment, row monitoring.Collection) (map[string]float64, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	expression := "mysql_uptime"
	if row.Integration == "sqlserver" {
		expression = "sqlserver_user_connection_count"
	}
	value, err := client.JSON(ctx, "plan8-service-metric-"+row.ID.String(), "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": expression, "resource_ids": []string{row.Target.ID.String()}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
	if err != nil {
		return nil, err
	}
	sources := map[string]float64{}
	var walk func(any) error
	walk = func(value any) error {
		switch value := value.(type) {
		case []any:
			for _, item := range value {
				if err := walk(item); err != nil {
					return err
				}
			}
		case map[string]any:
			if labels, ok := value["metric"].(map[string]any); ok {
				if labels["argus_resource_id"] != row.Target.ID.String() {
					return errors.New("service metric escaped its target")
				}
				if pair, ok := value["value"].([]any); ok && len(pair) == 2 {
					id, _ := labels["argus_collection_id"].(string)
					if id != row.ID.String() {
						return errors.New("service metric has no trusted logical collection")
					}
					n, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
					if err != nil || n <= 0 {
						return errors.New("service did not return a real positive observation")
					}
					sources[id] += n
				}
			}
			for _, item := range value {
				if err := walk(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(value); err != nil {
		return nil, err
	}
	if len(sources) != 1 {
		return nil, fmt.Errorf("service has %d instant-query logical identities instead of one collection", len(sources))
	}
	return sources, nil
}

func (a *App) plan8CurrentCollectionSource(ctx context.Context, env *E2EEnvironment, row monitoring.Collection) (uuid.UUID, error) {
	enterprise := env.State.Values["enterprise_id"]
	if uuid.Validate(enterprise) != nil {
		return uuid.Nil, errors.New("invalid acceptance enterprise")
	}
	query := fmt.Sprintf("SELECT s.id FROM telemetry_sources s JOIN monitoring_collections c ON c.id=s.collection_id AND c.enterprise_id=s.enterprise_id JOIN collector_instances ci ON ci.id=s.collector_id AND ci.enterprise_id=s.enterprise_id WHERE c.enterprise_id='%s' AND c.id='%s' AND c.effective_collector_id=s.collector_id AND c.effective_assignment_epoch=s.assignment_epoch AND c.effective_revision=s.collection_revision AND ci.effective_revision=s.config_revision AND ci.source_generation=s.generation", enterprise, row.ID)
	value, err := a.postgresQuery(ctx, env, query)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(strings.TrimSpace(value))
}

func (a *App) plan8DeployedServices(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	if err := a.plan8WaitCollectorPaths(ctx, env); err != nil {
		a.plan8ServiceReadinessEvidence(env, "control-readiness")
		return err
	}
	fixtures, err := a.preparePlan8Databases(ctx, env, h)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		failure = errors.Join(failure, a.cleanupPlan8Databases(cleanup, fixtures))
	}()
	if err != nil {
		return err
	}
	leaf, err := uuid.Parse(h.CollectorID)
	if err != nil {
		return err
	}
	rows := []monitoring.Collection{}
	for index, target := range fixtures {
		port := "3306"
		if target.engine == "sqlserver" {
			port = "1433"
		}
		id, err := a.plan8ServiceCommit(ctx, env, fmt.Sprintf("create-%d", index), map[string]any{"kind": "collection", "operation": "create", "collection": map[string]any{"name": target.name, "target_name": target.name, "integration": target.engine, "target_identity": map[string]string{"endpoint": target.name + ":" + port}, "collector_id": leaf, "config": map[string]string{"interval": "5s", "timeout": "5s"}, "enabled": true, "credential_update": "replace"}, "credentials": map[string]string{"username": "monitor", "password": target.password}})
		if err != nil {
			return err
		}
		row, err := a.plan8ServiceEffective(ctx, env, id, leaf, true, nil)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	managedViews := []map[string]any{}
	seen := map[string]bool{}
	for _, row := range rows {
		dashboard, viewErr := a.plan8ExtraManagedDashboard(ctx, env, row)
		if viewErr != nil {
			return viewErr
		}
		managedViews = append(managedViews, map[string]any{"collection_id": row.ID, "integration": row.Integration, "dashboard": redactJSON(dashboard)})
		sources, err := a.plan8ServiceMetric(ctx, env, row)
		if err != nil {
			return err
		}
		for id := range sources {
			if seen[id] {
				return errors.New("different database targets shared a source identity")
			}
			seen[id] = true
		}
	}
	privateCanaries := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		privateCanaries = append(privateCanaries, fixture.password)
	}
	if err = a.plan8ModelDisableInstance(ctx, env, rows[0], rows[1], privateCanaries); err != nil {
		return err
	}
	rows[0], err = a.plan8ServiceCollection(ctx, env, rows[0].ID)
	if err != nil {
		return err
	}
	rows[0], err = a.plan8ServiceUpdate(ctx, env, "resume-after-model", rows[0], leaf, true)
	if err != nil {
		return err
	}
	rows[0], err = a.plan8ServiceUpdate(ctx, env, "stop-one", rows[0], leaf, false)
	if err != nil {
		return err
	}
	for index := 1; index < len(rows); index++ {
		rows[index], err = a.plan8ServiceEffective(ctx, env, rows[index].ID, leaf, true, rows[index].LastSampleAt)
		if err != nil {
			return err
		}
		if _, err = a.plan8ServiceMetric(ctx, env, rows[index]); err != nil {
			return err
		}
	}
	rows[0], err = a.plan8ServiceUpdate(ctx, env, "resume-one", rows[0], leaf, true)
	if err != nil {
		return err
	}
	if uuid.Validate(env.State.Values["enterprise_id"]) != nil {
		return errors.New("invalid acceptance enterprise")
	}
	gatewayText, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT gateway_collector_id FROM telemetry_routes WHERE collector_id='%s' AND enterprise_id='%s'", leaf, env.State.Values["enterprise_id"]))
	if err != nil {
		return err
	}
	gateway, err := uuid.Parse(strings.TrimSpace(gatewayText))
	if err != nil {
		return err
	}
	beforeMove, err := a.plan8CurrentCollectionSource(ctx, env, rows[0])
	if err != nil {
		return err
	}
	rows[0], err = a.plan8ServiceUpdate(ctx, env, "move-to-gateway", rows[0], gateway, true)
	if err != nil {
		return err
	}
	if _, err = a.plan8ServiceMetric(ctx, env, rows[0]); err != nil {
		return err
	}
	afterMove, err := a.plan8CurrentCollectionSource(ctx, env, rows[0])
	if err != nil {
		return err
	}
	if beforeMove == afterMove {
		return errors.New("migration did not replace the authenticated physical source")
	}
	retirementSQL := fmt.Sprintf("SELECT count(*) FROM monitoring_assignment_retirements WHERE enterprise_id='%s' AND collection_id='%s' AND previous_collector_id='%s' AND status='completed'", env.State.Values["enterprise_id"], rows[0].ID, leaf)
	if err := a.waitPostgresValue(ctx, env, retirementSQL, "1", 2*time.Minute); err != nil {
		return fmt.Errorf("old execution retirement was not actually observed: %w", err)
	}
	for index := range rows {
		rows[index], err = a.plan8ServiceUpdate(ctx, env, fmt.Sprintf("all-off-%d", index), rows[index], rows[index].CollectorID, false)
		if err != nil {
			return err
		}
	}
	if _, err := a.waitP4HostCollector(ctx, env, h.HostID, "converged"); err != nil {
		return err
	}
	rootHost, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT resource_id FROM collector_instances WHERE enterprise_id='%s' AND id='%s' AND resource_type='host'", env.State.Values["enterprise_id"], gateway))
	if err != nil {
		return err
	}
	rootHost = strings.TrimSpace(rootHost)
	if uuid.Validate(rootHost) != nil {
		return errors.New("gateway execution host missing")
	}
	if _, err := a.waitP4HostCollector(ctx, env, rootHost, "converged"); err != nil {
		return err
	}
	if err := a.plan8FreshHostSample(ctx, env, h); err != nil {
		return err
	}
	if _, err := a.plan6HostMetric(ctx, env, h, "after-service-all-off", h.CPUExpression); err != nil {
		return err
	}
	body, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "actual_two_mysql_sqlserver_one_collector_independent_stop_resume_and_two_collector_migration", "collections": rows, "managed_default_views": managedViews, "synthetic_metric_injection": false}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-services.json"), body)
}
