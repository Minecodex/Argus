package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

type plan11AuthoringCase struct {
	Name         string `json:"name"`
	Conversation string `json:"conversation_id"`
	Draft        string `json:"draft_id"`
	Action       string `json:"action_ref"`
	Decision     string `json:"decision"`
}

// Publication is deliberately performed by the actual Chatbox confirmation UI,
// after the model has produced a validated private draft and an immutable preview.
func (a *App) plan11DeployedAuthoring(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if env.Options.Plan8Phase != "plan11-publish" && env.Options.Plan8Phase != "plan11-navigation-publish" && env.Options.Plan8Phase != "plan11-authoring" || host == nil || env.Options.RealModel == nil {
		return errors.New("authoring acceptance requires its explicit owned real-model phase")
	}
	proof := map[string]any{"passed": false, "whole_plan_complete": false}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	proof["model_id"] = model
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		failure = errors.Join(failure, a.plan11AuthoringEvidence(cleanup, env, model, proof))
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, e := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, e, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring.json"), raw))
	}()
	cases := []plan11AuthoringCase{}
	for _, locale := range []string{"zh", "en"} {
		row, e := a.plan11AuthoringTask(ctx, env, host, model, locale)
		if e != nil {
			failure = errors.Join(failure, e)
			if errors.Is(e, errPlan11NavigationBoundary) || ctx.Err() != nil {
				return failure
			}
			continue
		}
		cases = append(cases, row)
	}
	proof["cases"] = cases
	if len(cases) != 2 {
		return errors.Join(failure, errors.New("both actual authoring previews are required before UI decisions"))
	}
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return errors.Join(failure, err)
	}
	raw, _ := json.Marshal(cases)
	if err = a.runPlaywright(ctx, env, "plan11-authoring-real.spec.ts", map[string]string{
		"ARGUS_PLAN11_AUTHORING_REAL": "1", "ARGUS_PLAN11_AUTHORING_CASES": string(raw),
		"ARGUS_PLAN11_AUTHORING_COOKIES": cookies, "ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-authoring",
	}); err != nil {
		return errors.Join(failure, err)
	}
	for _, row := range cases {
		if err = a.plan11AuthoringFinalState(ctx, env, row); err != nil {
			failure = errors.Join(failure, err)
		}
	}
	return failure
}

func (a *App) plan11AuthoringTask(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop, model, locale string) (row plan11AuthoringCase, failure error) {
	name := "Plan11 publication " + locale + " " + uuid.NewString()
	conversation, err := a.plan7ModelConversation(ctx, env, model, "plan11-authoring-"+locale)
	if err != nil {
		return row, err
	}
	row = plan11AuthoringCase{Name: name, Conversation: conversation, Decision: "cancel"}
	if locale == "en" {
		row.Decision = "confirm"
	}
	prompt := fmt.Sprintf("请为授权主机%q（ID %s）新建名为%q的个人仪表盘草稿，默认最近30分钟，只需CPU忙碌率和已用内存百分比两张趋势图，使用该主机实际的hostmetrics来源，建议关联到这台主机快捷入口。先确认实际目录/来源；复用编辑器校验并检查草稿样本，修正错误，再提供发布预览，等待我在界面确认。不要直接发布、执行远程操作或修改其他配置。用中文简要说明预览状态和样本限制。", host.HostName, host.HostID, name)
	if locale == "en" {
		prompt = fmt.Sprintf("Create a new private dashboard draft named %q for authorized host %q (ID %s), default last30minutes. Include only two trend panels: CPU busy percentage and used memory percentage, using its actual hostmetrics source. Propose a shortcut binding to this Host. Verify the actual catalog/sources, validate with the editor and check draft samples, repair errors, then provide a publication preview and wait for my UI confirmation. Do not publish directly, run remote operations or change other configuration. Briefly explain preview status and sample limitations in English.", name, host.HostName, host.HostID)
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	sample, err := a.realModelTask(bounded, env, conversation, "plan11-authoring-"+locale, prompt, nil, map[string]any{"mode": "create", "dashboard_ids": []string{}})
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if sample.RunID != "" {
			failure = errors.Join(failure, a.planV2ModelEvidence(cleanup, env, conversation, sample, failure))
			if failure != nil {
				failure = errors.Join(failure, a.plan8StopFailedModelTask(cleanup, env, sample.RunID))
			}
		}
	}()
	if err != nil {
		return row, err
	}
	if sample.ModelCalls < 1 || sample.ModelCalls > 32 || !sample.UsageComplete || !sample.CachedUsageComplete {
		return row, errors.New("authoring model accounting incomplete or over bound")
	}
	trace, err := a.postgresQuery(bounded, env, plan11InteractionTraceSQL(sample.RunID))
	if err != nil {
		return row, err
	}
	if err = plan11AuthoringTraceAllowed(trace); err != nil {
		return row, err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return row, err
	}
	drafts, err := client.JSONArray(bounded, "p11-authoring-drafts-"+locale, "enterprise", http.MethodGet, "/dashboard-drafts", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return row, err
	}
	for _, draft := range drafts {
		if draft["name"] != name {
			continue
		}
		if row.Draft != "" || draft["dashboard_id"] != nil || draft["status"] == "published" {
			return row, errors.New("authoring bypassed unique private draft")
		}
		raw, _ := json.Marshal(draft["spec"])
		spec, e := dashboard.DecodeSpec(raw)
		if e != nil || !plan11AuthoringSpecMatches(spec) {
			return row, errors.Join(e, errors.New("draft does not match requested two-panel Host dashboard"))
		}
		bindings, _ := json.Marshal(draft["proposed_bindings"])
		var actual []dashboard.Binding
		if json.Unmarshal(bindings, &actual) != nil || !planV2BindingsEqual(actual, []dashboard.Binding{{ResourceType: "host", ResourceID: uuid.MustParse(host.HostID)}}) {
			return row, errors.New("draft Host shortcut differs from requested authorized resource")
		}
		row.Draft, _ = draft["id"].(string)
	}
	if uuid.Validate(row.Draft) != nil {
		return row, errors.New("model did not leave a valid private draft")
	}
	actions, err := a.postgresQuery(bounded, env, "SELECT action_ref FROM pending_actions WHERE run_id='"+sample.RunID+"' AND action_type='telemetry.dashboard.publish' AND status='awaiting_confirmation'")
	if err != nil || len(strings.Fields(actions)) != 1 {
		return row, errors.Join(err, errors.New("model did not leave exactly one pending publication preview"))
	}
	row.Action = strings.TrimSpace(actions)
	if err = a.plan11AuthoringBeforeConfirmation(bounded, env, sample.RunID, host.HostID, row); err != nil {
		return row, err
	}
	sample.Passed = true
	return row, nil
}

func plan11AuthoringSpecMatches(spec dashboard.Spec) bool {
	if !dashboard.Validate(spec).Valid || len(spec.Panels) != 2 || len(spec.Variables) != 0 || spec.DefaultTimeRange.Kind != "relative" || spec.DefaultTimeRange.Seconds != 1800 {
		return false
	}
	cpu, memory := false, false
	for _, p := range spec.Panels {
		if p.Type != "timeseries" || p.Signal != "metrics" || p.SourceBinding.SourceType != "hostmetrics" || len(p.Targets) != 1 {
			return false
		}
		raw, _ := json.Marshal(p.Targets[0].SourceDefinition)
		cpu = cpu || strings.Contains(string(raw), "system_cpu_utilization")
		memory = memory || strings.Contains(string(raw), "system_memory_utilization")
	}
	return cpu && memory
}

func plan11AuthoringTraceAllowed(raw string) error {
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return errors.New("invalid authoring trace")
	}
	for _, c := range calls {
		if slices.Contains([]string{"tool.search", "tool.describe", "read", "write", "edit", "bash"}, c.Tool) {
			continue
		}
		if c.Tool == "tool.invoke" {
			category, name := c.Input.Category, c.Input.Name
			if category == "host" && slices.Contains([]string{"list", "get"}, name) ||
				category == "observe" && slices.Contains([]string{"resources.list", "capabilities.get", "catalog.query"}, name) ||
				category == "dashboard" && slices.Contains([]string{"list", "get", "catalog", "catalog.resources", "draft.create", "draft.get", "draft.save", "draft.validate", "publish.preview"}, name) {
				continue
			}
		}
		return errPlan11NavigationBoundary
	}
	return nil
}
