package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

type plan8ExtraTarget struct {
	plan8DatabaseFixture
	endpoint, metric string
	database         string
	config           map[string]any
	identity         map[string]any
}

func (a *App) preparePlan8ExtraServices(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) ([]plan8ExtraTarget, error) {
	network := ""
	for _, n := range h.Docker.Networks {
		if n.Internal {
			network = n.Name
		}
	}
	if network == "" {
		return nil, errors.New("owned customer network missing")
	}
	owner := "plan8-extra-" + env.Options.RunID
	rows := []plan8ExtraTarget{}
	save := func() error {
		public := []map[string]string{}
		for _, r := range rows {
			public = append(public, map[string]string{"owner": r.owner, "name": r.name, "id": r.id, "engine": r.engine, "image_id": r.imageID, "target_version": r.version})
		}
		data, _ := json.MarshalIndent(map[string]any{"network": network, "containers": public}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-extra-services-ownership.json"), data)
	}
	for _, engine := range []string{"redis", "nginx"} {
		row := plan8ExtraTarget{plan8DatabaseFixture: plan8DatabaseFixture{owner: owner, name: kubernetesNameForDev(owner + "-" + engine), engine: engine}}
		image := "nginx:1.28-alpine"
		containerPath := "/etc/nginx/nginx.conf"
		contents := "worker_processes 1; events {worker_connections 128;} http {server {listen 8080; location /status {stub_status;}}}"
		row.endpoint = "http://" + row.name + ":8080/status"
		row.metric = "nginx_connections_accepted"
		command := []string{}
		if engine == "redis" {
			row.password = "P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			image = "redis:7.4-alpine"
			containerPath = "/redis.conf"
			row.endpoint = row.name + ":6379"
			row.metric = "redis_uptime"
			contents = "bind 0.0.0.0\nprotected-mode no\nappendonly no\nsave \"\"\nuser default off\nuser monitor reset on >" + row.password + " -@all +info +ping +hello +cluster|info +client|setinfo\n"
			command = []string{"redis-server", "/redis.conf"}
		}
		config := filepath.Join(env.WorkDir, "plan8-extra-"+engine+".conf")
		if err := os.WriteFile(config, []byte(contents), 0600); err != nil {
			return rows, err
		}
		arguments := []string{"run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run=" + owner, "--network", network, "--memory=96m", "--cpus=.5", "--mount", "type=bind,source=" + config + ",target=" + containerPath + ",readonly", image}
		arguments = append(arguments, command...)
		// The create intent is recoverable even if Docker starts the container but
		// the caller loses the response before receiving its ID.
		rows = append(rows, row)
		if err := save(); err != nil {
			return rows, err
		}
		id, err := a.runner.Output(ctx, nil, "docker", arguments...)
		if err != nil {
			return rows, fmt.Errorf("owned %s start failed", engine)
		}
		row.id = strings.TrimSpace(id)
		rows[len(rows)-1] = row
		if err = save(); err != nil {
			return rows, err
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			var buffer bytes.Buffer
			if engine == "redis" {
				err = a.runner.RunIO(ctx, map[string]string{"REDISCLI_AUTH": row.password}, nil, &buffer, io.Discard, "docker", "exec", "--env", "REDISCLI_AUTH", row.id, "redis-cli", "--user", "monitor", "--no-auth-warning", "-e", "PING")
			} else {
				err = a.runner.RunIO(ctx, nil, nil, &buffer, io.Discard, "docker", "exec", row.id, "wget", "-q", "-O", "-", "http://127.0.0.1:8080/status")
			}
			if err == nil && ((engine == "redis" && strings.Contains(buffer.String(), "PONG")) || (engine == "nginx" && strings.Contains(buffer.String(), "Active connections"))) {
				break
			}
			if time.Now().After(deadline) {
				return rows, fmt.Errorf("owned %s readiness failed", engine)
			}
			if err = waitContext(ctx, 200*time.Millisecond); err != nil {
				return rows, err
			}
		}
		imageID, inspectErr := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
		if inspectErr != nil || !strings.HasPrefix(strings.TrimSpace(imageID), "sha256:") {
			return rows, errors.New("owned service image identity not observable")
		}
		versionCommand := []string{"exec", row.id, "nginx", "-v"}
		if engine == "redis" {
			versionCommand = []string{"exec", row.id, "redis-server", "--version"}
		}
		var version bytes.Buffer
		if err = a.runner.RunIO(ctx, nil, nil, &version, &version, "docker", versionCommand...); err != nil || version.Len() == 0 || version.Len() > 256 {
			return rows, errors.New("owned service runtime version not observable")
		}
		row.imageID, row.version = strings.TrimSpace(imageID), strings.TrimSpace(version.String())
		rows[len(rows)-1] = row
		if err = save(); err != nil {
			return rows, err
		}
		if engine == "redis" {
			for _, operation := range [][]string{{"SET", "forbidden", "value"}, {"GET", "forbidden"}, {"CONFIG", "GET", "requirepass"}} {
				arguments := append([]string{"exec", "--env", "REDISCLI_AUTH", row.id, "redis-cli", "--user", "monitor", "--no-auth-warning", "-e"}, operation...)
				var response bytes.Buffer
				err = a.runner.RunIO(ctx, map[string]string{"REDISCLI_AUTH": row.password}, nil, &response, &response, "docker", arguments...)
				if err == nil || !strings.Contains(response.String(), "NOPERM") {
					return rows, errors.New("Redis monitoring role can access business values or protected configuration")
				}
			}
		}
	}
	return rows, nil
}

func plan8ExtraMetricValues(value any, target, collection string) ([]float64, error) {
	return plan8MetricValues(value, target, collection, false)
}

func plan8MetricValues(value any, target, collection string, scopedAggregation bool) ([]float64, error) {
	values := []float64{}
	var walk func(any) error
	walk = func(value any) error {
		switch value := value.(type) {
		case []any:
			for _, v := range value {
				if err := walk(v); err != nil {
					return err
				}
			}
		case map[string]any:
			if labels, ok := value["metric"].(map[string]any); ok {
				unlabeledAggregate := scopedAggregation && labels["argus_resource_id"] == nil && labels["argus_collection_id"] == nil
				if !unlabeledAggregate && (labels["argus_resource_id"] != target || labels["argus_collection_id"] != collection) {
					return errors.New("service query escaped its target or logical instance")
				}
				points := []any{}
				if pair, ok := value["value"].([]any); ok {
					points = append(points, pair)
				}
				if pairs, ok := value["values"].([]any); ok {
					points = append(points, pairs...)
				}
				for _, point := range points {
					pair, ok := point.([]any)
					if !ok || len(pair) != 2 {
						return errors.New("invalid service sample")
					}
					number, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
					if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
						return errors.New("service sample is not finite")
					}
					values = append(values, number)
				}
			}
			for _, v := range value {
				if err := walk(v); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err := walk(value)
	return values, err
}

func (a *App) plan8ExtraManagedDashboard(ctx context.Context, env *E2EEnvironment, row monitoring.Collection) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	list, err := client.JSONArray(ctx, "plan8-extra-dashboard-list-"+row.Integration, "enterprise", http.MethodGet, "/dashboards", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return nil, err
	}
	dashboard := ""
	for _, item := range list {
		if item["managed_template_key"] == row.Integration+".overview" {
			dashboard, _ = item["id"].(string)
		}
	}
	if dashboard == "" {
		return nil, errors.New("managed default dashboard not offered for the actual target")
	}
	deadline := time.Now().Add(45 * time.Second)
	expected, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(id::text || ':' || config_revision::text),'[]') FROM telemetry_sources WHERE collector_id='"+row.CollectorID.String()+"' AND collection_id='"+row.ID.String()+"' AND resource_id='"+row.Target.ID.String()+"'")
	if err != nil {
		return nil, err
	}
	var sourceKeys []string
	if json.Unmarshal([]byte(expected), &sourceKeys) != nil || len(sourceKeys) == 0 {
		return nil, errors.New("default view lacks actual registered collection sources")
	}
	for {
		result, err := client.JSON(ctx, "plan8-extra-dashboard-"+row.Integration, "enterprise", http.MethodPost, "/dashboards/"+dashboard+"/execute", 200, map[string]any{"resource_ids": []string{row.Target.ID.String()}}, enterpriseHeaders(env, uuid.NewString()))
		if err != nil {
			return nil, err
		}
		if err := plan8DefaultViewScope(result, row, sourceKeys); err != nil {
			body, _ := json.MarshalIndent(redactJSON(result), "", "  ")
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-extra-dashboard-"+row.Integration+"-failure.json"), body)
			return nil, err
		}
		complete := result["partial"] != true
		panels, _ := result["panels"].([]any)
		if len(panels) == 0 {
			complete = false
		}
		for _, item := range panels {
			panel, _ := item.(map[string]any)
			targets, _ := panel["targets"].([]any)
			if panel["status"] != "success" || len(targets) == 0 {
				complete = false
				continue
			}
			for _, raw := range targets {
				target, _ := raw.(map[string]any)
				panelID, _ := panel["id"].(string)
				targetID, _ := target["id"].(string)
				values, e := plan8MetricValues(target["data"], row.Target.ID.String(), row.ID.String(), plan8DefaultTargetAggregates(row.Integration, panelID, targetID))
				if e != nil {
					body, _ := json.MarshalIndent(redactJSON(result), "", "  ")
					_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-extra-dashboard-"+row.Integration+"-failure.json"), body)
					return nil, e
				}
				if target["status"] != "success" || len(values) == 0 {
					complete = false
				}
			}
		}
		if complete {
			return result, nil
		}
		if time.Now().After(deadline) {
			body, _ := json.MarshalIndent(redactJSON(result), "", "  ")
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-extra-dashboard-"+row.Integration+"-timeout.json"), body)
			return nil, errors.New("managed default dashboard did not receive actual scoped panel samples")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return nil, err
		}
	}
}

func (a *App) plan8DeployedExtraServices(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	fixtures, err := a.preparePlan8ExtraServices(ctx, env, h)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		rows := []plan8DatabaseFixture{}
		for _, row := range fixtures {
			rows = append(rows, row.plan8DatabaseFixture)
		}
		failure = errors.Join(failure, a.cleanupPlan8Databases(cleanup, rows))
	}()
	if err != nil {
		return err
	}
	collector, err := uuid.Parse(h.CollectorID)
	if err != nil {
		return err
	}
	records := []map[string]any{}
	for _, fixture := range fixtures {
		record, err := a.plan8VerifyDedicatedTarget(ctx, env, collector, fixture, false)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "enrolled_collector_api_actual_redis_nginx_private_credentials_managed_dashboards_and_lifecycle", "resources": records}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-extra-services.json"), proof)
}

func (a *App) plan8VerifyDedicatedTarget(ctx context.Context, env *E2EEnvironment, collector uuid.UUID, fixture plan8ExtraTarget, verifyAuthorization bool) (record map[string]any, failure error) {
	created := uuid.Nil
	defer func() {
		if failure == nil || created == uuid.Nil {
			return
		}
		// Retire this exact failed instance before its owned fixture disappears.
		// The group retains historical receiver diagnostics before Docker cleanup.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		row, err := a.plan8ServiceCollection(cleanup, env, created)
		if err == nil && (row.Name != fixture.name || row.Integration != fixture.engine || row.CollectorID != collector) {
			err = errors.New("failed admission instance ownership changed")
		}
		if err == nil && row.DesiredEnabled {
			_, err = a.plan8ServiceUpdate(cleanup, env, "failed-admission-stop-"+fixture.engine, row, collector, false)
		}
		if err != nil {
			failure = errors.Join(failure, errPlan8TargetCleanup, err)
		}
	}()
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	identity := map[string]any{"endpoint": fixture.endpoint}
	if fixture.identity != nil {
		identity = fixture.identity
	}
	if fixture.database != "" {
		identity["database"] = fixture.database
	}
	config := fixture.config
	if config == nil {
		config = map[string]any{"interval": "5s", "timeout": "5s"}
	}
	collection := map[string]any{"name": fixture.name, "target_name": fixture.name, "integration": fixture.engine, "target_identity": identity, "collector_id": collector, "config": config, "enabled": true, "credential_update": "keep"}
	input := map[string]any{"kind": "collection", "operation": "create", "collection": collection}
	if fixture.password != "" {
		collection["credential_update"] = "replace"
		input["credentials"] = map[string]string{"username": "monitor", "password": fixture.password}
	}
	configuration, err := a.plan8DedicatedConfigAdmission(ctx, env, collector, fixture.engine, input)
	if err != nil {
		return nil, err
	}
	id, err := a.plan8ServiceCommit(ctx, env, "extra-create-"+fixture.engine, input)
	if err != nil {
		return nil, err
	}
	created = id
	row, err := a.plan8ServiceEffective(ctx, env, id, collector, true, nil)
	if err != nil {
		return nil, err
	}
	// Ingest acknowledgement is earlier than Writer/query visibility. Wait
	// for this actual metric at the same bounded query-readiness boundary used
	// by the default view, without accepting another target or source instance.
	var samples []float64
	deadline := time.Now().Add(45 * time.Second)
	for {
		value, err := client.JSON(ctx, "plan8-extra-metric-"+fixture.engine, "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": fixture.metric, "resource_ids": []string{row.Target.ID.String()}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		samples, err = plan8ExtraMetricValues(value, row.Target.ID.String(), row.ID.String())
		if err != nil {
			return nil, err
		}
		if plan8DedicatedSamplesReady(fixture.engine, samples) {
			body, _ := json.MarshalIndent(redactJSON(value), "", "  ")
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-extra-metric-"+fixture.engine+".json"), body)
			break
		}
		if time.Now().After(deadline) {
			body, _ := json.MarshalIndent(redactJSON(value), "", "  ")
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-extra-metric-"+fixture.engine+"-timeout.json"), body)
			return nil, errors.New("dedicated metric was not queryable after actual Ingest acknowledgement: " + fixture.metric)
		}
		if err := waitContext(ctx, 500*time.Millisecond); err != nil {
			return nil, err
		}
	}
	dashboard, err := a.plan8ExtraManagedDashboard(ctx, env, row)
	if err != nil {
		return nil, err
	}
	authorization := map[string]any{"status": "not_run_in_foundation_stage", "following_gate": "existing-target-authorization"}
	if verifyAuthorization {
		authorization, err = a.plan8VerifyTargetAuthorization(ctx, env, row, fixture.metric)
		if err != nil {
			return nil, err
		}
	}
	row, err = a.plan8ServiceUpdate(ctx, env, "extra-disable-"+fixture.engine, row, collector, false)
	if err != nil {
		return nil, err
	}
	row, err = a.plan8ServiceUpdate(ctx, env, "extra-resume-"+fixture.engine, row, collector, true)
	if err != nil {
		return nil, err
	}
	row, err = a.plan8ServiceUpdate(ctx, env, "extra-final-stop-"+fixture.engine, row, collector, false)
	if err != nil {
		return nil, err
	}
	return map[string]any{"integration": fixture.engine, "collection": row, "configuration_gate": configuration, "actual_samples": samples, "default_dashboard": redactJSON(dashboard), "independent_disable_resume_verified": true, "target_runtime_version": fixture.version, "target_image_id": fixture.imageID, "platform_target_authorization": authorization}, nil
}

// A loss sample alone does not establish the healthy ICMP fixture required by
// the positive RTT view gate. Actual loss/recovery is verified separately;
// the product template must continue suppressing RTT when all packets are lost.
func plan8DedicatedSamplesReady(integration string, samples []float64) bool {
	if len(samples) == 0 {
		return false
	}
	if integration == "icmp" {
		for _, loss := range samples {
			if loss != 0 {
				return false
			}
		}
	}
	return true
}
