package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

const plan11IdentityTargetName = "Plan11 same-address database"
const plan11IdentityApplicationName = "Plan11 same-name service"
const plan11IdentityEndpoint = "10.77.0.8:3306"

func (a *App) plan11DeployedResourceIdentity(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if env.Options.Plan8Phase != "plan11-identity" || env.Options.RealModel == nil {
		return errors.New("resource identity acceptance requires its selected real-provider phase")
	}
	proof := map[string]any{"schema_version": "argus.plan11_resource_identity/v1", "passed": false,
		"decision":      "D33: explicit resource identity plus authorized placement; ask when intent is insufficient",
		"scope":         "actual registered Host/Collector placements; disabled database configurations and application metadata, no database connectivity or telemetry claim",
		"planned_tasks": plan8SelectedModelNames("plan11-identity"), "provider_call_acceptance_limit_total": 384,
		"provider_probe_calls_separate": true, "whole_plan_complete": false}
	var cases []map[string]any
	defer func() {
		proof["cases"], proof["passed"] = cases, failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, err := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, err, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-resource-identity.json"), raw))
	}()
	if uuid.Validate(host.HostID) != nil || uuid.Validate(host.RootHostID) != nil || host.HostID == host.RootHostID {
		return errors.New("identity fixture requires two actual distinct authorized Host placements")
	}
	collector, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT id FROM collector_instances WHERE enterprise_id='%s' AND resource_type='host' AND resource_id='%s' AND role='edge_gateway' AND status='converged'`, env.State.Values["enterprise_id"], host.RootHostID))
	if err != nil || uuid.Validate(strings.TrimSpace(collector)) != nil || strings.TrimSpace(collector) == host.CollectorID {
		return errors.Join(err, errors.New("identity fixture has no distinct converged gateway Collector"))
	}
	other := &plan6HostLoop{CollectorID: strings.TrimSpace(collector)}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	rootHost, err := client.JSON(ctx, "identity-authorized-root-host", "enterprise", http.MethodGet, "/enterprise/hosts/"+host.RootHostID, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	rootName, err := stringField(rootHost, "name")
	if err != nil || rootName == "" || rootName == host.HostName {
		return errors.Join(err, errors.New("authorized fixture Hosts have no distinct placement names"))
	}
	first, err := a.plan11NavigationFixture(ctx, env, host, plan11IdentityTargetName, plan11IdentityEndpoint, "identity-candidate-17")
	if err != nil {
		return err
	}
	second, err := a.plan11NavigationFixture(ctx, env, other, plan11IdentityTargetName, plan11IdentityEndpoint, "identity-candidate-42")
	if err != nil || first.Target.ID == second.Target.ID || first.CollectorID == second.CollectorID {
		return errors.Join(err, errors.New("same-address targets or actual Collector placements were merged"))
	}
	apps := map[string]uuid.UUID{}
	for _, environment := range []string{"production", "testing"} {
		id, err := a.plan8ServiceCommit(ctx, env, "identity-app-"+environment, map[string]any{"kind": "application", "operation": "create", "application": map[string]any{"name": plan11IdentityApplicationName, "environment": environment}})
		if err != nil {
			return err
		}
		apps[environment] = id
	}
	if apps["production"] == apps["testing"] {
		return errors.New("same-name applications lost their environment identities")
	}
	proof["fixtures"] = map[string]any{"first": first, "second": second, "first_host_id": host.HostID, "selected_host_id": host.RootHostID, "selected_host_name": rootName, "endpoint": plan11IdentityEndpoint, "applications": apps}
	for _, locale := range []string{"zh", "en"} {
		for repeat := 1; repeat <= 3; repeat++ {
			name := fmt.Sprintf("plan11-placement-%s-%d", locale, repeat)
			row, err := a.plan11PreparedMetadataTask(ctx, env, name, plan11PlacementPrompt(locale, host.RootHostID), nil, plan11IdentityTraceAllowed,
				func(ctx context.Context, conversation string, sample *p5BenchmarkSample, row map[string]any) error {
					return a.plan11VerifyPlacement(ctx, env, sample, second, apps["production"], host.RootHostID, row)
				})
			cases = append(cases, row)
			if err != nil {
				return err
			}
			name = fmt.Sprintf("plan11-identity-ask-%s-%d", locale, repeat)
			row, err = a.plan11PreparedMetadataTask(ctx, env, name, plan11SameAddressQuestionPrompt(locale), nil, plan11IdentityTraceAllowed,
				func(ctx context.Context, conversation string, sample *p5BenchmarkSample, row map[string]any) error {
					return a.plan11ResolveAmbiguityWithFacts(ctx, env, conversation, sample, first, second, row,
						[]string{host.HostID, host.HostName, first.ID.String(), first.Name}, []string{host.RootHostID, rootName, second.ID.String(), second.Name},
						"Use only monitoring target "+second.Target.ID.String()+". Inspect configuration only; do not query telemetry or make changes.")
				})
			cases = append(cases, row)
			if err != nil {
				return err
			}
		}
	}
	return a.plan11WaitModelAccounting(ctx, env)
}

func plan11PlacementPrompt(locale, host string) string {
	if locale == "zh" {
		return fmt.Sprintf("只读核对生产环境名为 %q 的应用，以及由已授权主机 %s 上采集器负责的 MySQL 目标 %q（地址 %s）。同名应用可能在其他环境，同地址目标可能有其他采集归属，请根据实际身份和已授权归属选择，不按第一条猜测。不修改配置、不查询遥测、不探测数据库。完成后简述依据，并用一个 JSON 代码块给出 application_id、resource_id、host_id、collection_enabled、sampling_performed。请用中文回答。", plan11IdentityApplicationName, host, plan11IdentityTargetName, plan11IdentityEndpoint)
	}
	return fmt.Sprintf("Read-only inspect the production application named %q and the MySQL target %q (address %s) collected by the Collector on authorized Host %s. Same-name applications may belong to other environments, and same-address targets may have other placements. Resolve actual identities and authorized placement; never pick the first row. Do not change configuration, query telemetry or probe the database. Briefly explain the evidence and provide one JSON block containing application_id, resource_id, host_id, collection_enabled, sampling_performed. Answer in English.", plan11IdentityApplicationName, plan11IdentityTargetName, plan11IdentityEndpoint, host)
}

func plan11SameAddressQuestionPrompt(locale string) string {
	if locale == "zh" {
		return fmt.Sprintf("请只读查看 MySQL 目标 %q（地址 %s）的当前采集配置和可用能力。先确认具体对象，有歧义时向我询问，不自行选第一条。不修改配置、不查询遥测、不探测数据库。确认后用 JSON 代码块给出 resource_id、collection_enabled、sampling_performed。请用中文回答。", plan11IdentityTargetName, plan11IdentityEndpoint)
	}
	return fmt.Sprintf("Inspect only the current collection configuration and capabilities of the MySQL target %q at %s. Resolve the exact object first and ask me if ambiguous; never pick the first row. Do not change configuration, query telemetry or probe the database. After clarification provide a JSON block with resource_id, collection_enabled, sampling_performed. Answer in English.", plan11IdentityTargetName, plan11IdentityEndpoint)
}

func plan11IdentityTraceAllowed(raw string) error {
	var calls []struct {
		Tool  string
		Input struct{ Category, Name string }
	}
	if json.Unmarshal([]byte(raw), &calls) != nil || len(calls) == 0 {
		return errors.New("identity navigation trace missing")
	}
	for _, call := range calls {
		if call.Tool == "tool.invoke" && call.Input.Category == "application" && (call.Input.Name == "list" || call.Input.Name == "get") {
			continue
		}
		if call.Tool == "tool.invoke" && call.Input.Category == "host" && (call.Input.Name == "list" || call.Input.Name == "get") {
			continue
		}
		if call.Tool == "tool.invoke" && call.Input.Category == "connector" && (call.Input.Name == "collector.list" || call.Input.Name == "collector.get") {
			continue
		}
		one, _ := json.Marshal([]any{call})
		if err := plan11NavigationTraceAllowed(string(one)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) plan11VerifyPlacement(ctx context.Context, env *E2EEnvironment, sample *p5BenchmarkSample, selected monitoring.Collection, app uuid.UUID, host string, proof map[string]any) error {
	if sample.RunStatus != "succeeded" {
		return errors.New("explicit environment/Host identity task did not resolve autonomously")
	}
	reply, err := a.planV2ModelReply(ctx, env, sample.RunID)
	if err != nil {
		return err
	}
	var result struct {
		Application string `json:"application_id"`
		Resource    string `json:"resource_id"`
		Host        string `json:"host_id"`
		Enabled     *bool  `json:"collection_enabled"`
		Sampled     *bool  `json:"sampling_performed"`
	}
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return errors.New("identity task omitted structured identity evidence")
	}
	text := reply[start+7:]
	end := strings.Index(text, "```")
	if end < 0 || json.Unmarshal([]byte(strings.TrimSpace(text[:end])), &result) != nil || result.Application != app.String() || result.Resource != selected.Target.ID.String() || result.Host != host || result.Enabled == nil || *result.Enabled || result.Sampled == nil || *result.Sampled {
		return errors.New("environment, placement or sampling conclusion differs from actual configuration")
	}
	state, err := a.postgresQuery(ctx, env, plan11NavigationStateReadSQL(sample.RunID, selected))
	if err != nil || strings.TrimSpace(state) == "0" {
		return errors.Join(err, errors.New("chosen collection configuration was not actually observed"))
	}
	observed, err := a.postgresQuery(ctx, env, plan11ApplicationIdentityReadSQL(sample.RunID, app))
	if err != nil || strings.TrimSpace(observed) == "0" {
		return errors.Join(err, errors.New("production application identity was not actually observed"))
	}
	placement, err := a.postgresQuery(ctx, env, plan11PlacementReadSQL(sample.RunID, selected.CollectorID, host))
	if err != nil || strings.TrimSpace(placement) == "0" {
		return errors.Join(err, errors.New("target-to-Collector-to-authorized-Host placement was not actually observed"))
	}
	proof["conclusion"] = result
	return nil
}

func plan11ApplicationIdentityReadSQL(run string, app uuid.UUID) string {
	return fmt.Sprintf(`SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id WHERE t.run_id='%s' AND t.status='succeeded' AND t.tool_id='tool.invoke' AND
 ((t.input->>'category'='application' AND t.input->>'name'='get' AND r.projection->'summary'->>'id'='%s' AND r.projection->'summary'->>'environment'='production')
 OR (((t.input->>'category'='application' AND t.input->>'name'='list') OR (t.input->>'category'='observe' AND t.input->>'name'='resources.list')) AND EXISTS(SELECT 1 FROM jsonb_array_elements(coalesce(r.projection->'summary'->'items','[]'::jsonb)) item WHERE item->>'id'='%s' AND item->>'environment'='production')))`, run, app, app)
}

func plan11PlacementReadSQL(run string, collector uuid.UUID, host string) string {
	return fmt.Sprintf(`SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id WHERE t.run_id='%s' AND t.status='succeeded' AND t.source='argus' AND t.tool_id='tool.invoke' AND
 ((t.input->>'category'='monitoring' AND t.input->>'name'='collector.choices' AND EXISTS(SELECT 1 FROM jsonb_array_elements(coalesce(r.projection->'summary'->'items','[]'::jsonb)) item WHERE item->>'id'='%s' AND item->'resource'->>'kind'='host' AND item->'resource'->>'id'='%s'))
 OR (t.input->>'category'='connector' AND t.input->>'name' IN ('collector.list','collector.get') AND EXISTS(SELECT 1 FROM jsonb_array_elements(coalesce(r.projection->'summary'->'items',jsonb_build_array(r.projection->'summary'->'collector'))) item WHERE item->>'id'='%s' AND item->>'resource_type'='host' AND item->>'resource_id'='%s')))`, run, collector, host, collector, host)
}
