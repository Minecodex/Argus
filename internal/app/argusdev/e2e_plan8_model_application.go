package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

type plan8ApplicationModelConclusion struct {
	Applications []struct {
		ID      string `json:"application_id"`
		Samples int    `json:"sample_count"`
		Errors  int    `json:"error_count"`
	} `json:"applications"`
	SharedResources []struct {
		ID          string `json:"resource_id"`
		CountedOnce bool   `json:"counted_once"`
	} `json:"shared_resources"`
	Limitations []string `json:"limitations"`
}

// Fixtures establish current relationships; the real provider independently
// discovers and queries the existing observations, then reads Workspace files.
func (a *App) plan8ModelApplicationObservation(ctx context.Context, env *E2EEnvironment) (failure error) {
	var sdk struct {
		Passed bool `json:"passed"`
		Cases  []struct {
			Protocol    string    `json:"protocol"`
			Application uuid.UUID `json:"application_id"`
			Environment string    `json:"environment"`
		} `json:"cases"`
	}
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-deployed-sdk.json"))
	if err != nil || json.Unmarshal(raw, &sdk) != nil || !sdk.Passed {
		return errors.New("application model requires this run's actual SDK evidence")
	}
	apps := []uuid.UUID{}
	for _, c := range sdk.Cases {
		if c.Protocol == "otlp_http" || c.Protocol == "otlp_grpc" {
			apps = append(apps, c.Application)
		}
	}
	if len(apps) != 2 || apps[0] == apps[1] || apps[0] == uuid.Nil || apps[1] == uuid.Nil {
		return errors.New("two distinct actual SDK applications required")
	}
	var services struct {
		Passed      bool                    `json:"passed"`
		Collections []monitoring.Collection `json:"collections"`
	}
	raw, err = os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-deployed-services.json"))
	if err != nil || json.Unmarshal(raw, &services) != nil || !services.Passed {
		return errors.New("shared resource requires this run's actual database evidence")
	}
	var shared monitoring.Collection
	for _, row := range services.Collections {
		if row.Integration == "mysql" {
			shared = row
			break
		}
	}
	if shared.Target.ID == uuid.Nil {
		return errors.New("actual shared MySQL target missing")
	}
	for i, app := range apps {
		if _, err = a.plan8ServiceCommit(ctx, env, fmt.Sprintf("model-shared-binding-%d", i), map[string]any{"kind": "binding", "operation": "create", "binding": map[string]any{"application_id": app, "target": shared.Target}}); err != nil {
			return err
		}
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	query, err := skywalking.ApplicationAPMQuery("services", 10)
	if err != nil {
		return err
	}
	expected := map[string]map[string]any{}
	for i, app := range apps {
		value, e := client.JSON(ctx, fmt.Sprintf("model-app-baseline-%d", i), "enterprise", http.MethodPost, "/enterprise/traces/graphql", 200, map[string]any{"query": query, "operation_name": "ApplicationAPM", "variables": map[string]any{"serviceIds": nil}, "resource_ids": []string{app.String()}, "time_range": telemetryTimeRange(30 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		data, _ := value["data"].(map[string]any)
		view, _ := data["queryAPMServices"].(map[string]any)
		rows, _ := view["rows"].([]any)
		if len(rows) != 1 {
			return errors.New("actual application baseline has missing or merged service identities")
		}
		row, _ := rows[0].(map[string]any)
		_, errorsPresent := row["errorCount"].(float64)
		if row["sampleCount"] != float64(2) || row["serviceName"] != "api" || !errorsPresent {
			return errors.New("actual SDK sample baseline changed")
		}
		expected[app.String()] = row
	}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "Plan8 applications and shared database")
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("请只读分析最近30分钟的两个应用 %s、%s，它们当前共享数据库 %s（资源ID %s）。两个应用可能有相同服务名，环境固定在各自应用上；请分别查询实际 APM 服务样本和错误数、业务日志及共享数据库历史指标，通过 Workspace 文件核对数据。没有某信号就明确说明，吞吐和延迟只能按接收到的样本解释，不能外推生产全量。共享数据库只作为一个依赖展示，不把其指标分摊成应用独占用量。不要修改配置或读取凭据。最后用 JSON 代码块归纳：{\"applications\":[{\"application_id\":\"实际ID\",\"sample_count\":实际样本数,\"error_count\":实际错误样本数}],\"shared_resources\":[{\"resource_id\":\"实际ID\",\"counted_once\":true}],\"limitations\":[\"实际样本和信号覆盖限制\"]}。", apps[0], apps[1], shared.Name, shared.Target.ID)
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	sample, err := a.realModelTask(bounded, env, conversation, "plan8-applications-shared-database", prompt, nil, nil)
	defer func() { failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, sample, failure)) }()
	if err != nil {
		return err
	}
	if uuid.Validate(sample.RunID) != nil || sample.RunStatus != "succeeded" || !sample.UsageComplete || !sample.CachedUsageComplete || sample.ModelCalls < 1 || sample.ModelCalls > 32 {
		return errors.New("application model usage or terminal state incomplete")
	}
	reply, err := a.planV2ModelReply(bounded, env, sample.RunID)
	if err != nil {
		return err
	}
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return errors.New("application model omitted structured observed conclusion")
	}
	value := reply[start+len("```json"):]
	end := strings.Index(value, "```")
	if end < 0 {
		return errors.New("application conclusion JSON was not completed")
	}
	var conclusion plan8ApplicationModelConclusion
	if json.Unmarshal([]byte(strings.TrimSpace(value[:end])), &conclusion) != nil {
		return errors.New("application conclusion JSON invalid")
	}
	if len(conclusion.Applications) != 2 || len(conclusion.SharedResources) != 1 || len(conclusion.Limitations) == 0 {
		return errors.New("application conclusion did not preserve resource separation or limits")
	}
	seen := map[string]bool{}
	for _, app := range conclusion.Applications {
		row, ok := expected[app.ID]
		if !ok || seen[app.ID] || app.Samples != int(row["sampleCount"].(float64)) || app.Errors != int(row["errorCount"].(float64)) {
			return errors.New("model application counts differ from actual independent scoped query")
		}
		seen[app.ID] = true
	}
	if conclusion.SharedResources[0].ID != shared.Target.ID.String() || !conclusion.SharedResources[0].CountedOnce {
		return errors.New("model counted shared dependency more than once or switched resource")
	}
	calls, err := a.postgresQuery(bounded, env, "SELECT coalesce(json_agg(json_build_object('tool',tool_id,'input',input,'status',status,'error_code',error_code) ORDER BY created_at,call_sequence),'[]') FROM tool_calls WHERE run_id='"+sample.RunID+"'")
	if err != nil {
		return err
	}
	if err = plan8VerifyApplicationModelQueries(calls, apps, shared.Target.ID); err != nil {
		return err
	}
	reads, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+sample.RunID+"' AND status='succeeded' AND tool_id IN ('read','bash')")
	if err != nil || strings.TrimSpace(reads) == "0" {
		return errors.New("application model did not read actual Workspace results")
	}
	mutations, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(mutations) != "0" {
		return errors.New("read-only application analysis prepared a mutation")
	}
	sample.Passed = true
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "replay": false, "sample": sample, "applications": apps, "shared_target": shared.Target, "independent_scoped_baselines": expected, "conclusion": conclusion, "scope": "current_relationships_real_apm_logs_shared_database_and_workspace_analysis_not_negative_authorization_gate"}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-model-applications-shared-database.json"), proof)
}
