package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

func (a *App) plan6ConfigurationFacts(ctx context.Context, env *E2EEnvironment) (string, error) {
	value, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(json_build_object('id',id,'kind',kind,'name',name,'version',version,'status',status,'spec',spec) ORDER BY id),'[]') FROM alert_configurations WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"';")
	return strings.TrimSpace(value), err
}

func (a *App) verifyPlan6ModelBusiness(ctx context.Context, env *E2EEnvironment, task, action, before string) error {
	after, err := a.plan6ConfigurationFacts(ctx, env)
	if err != nil {
		return err
	}
	fact, err := a.postgresQuery(ctx, env, "SELECT json_build_object('pending_action_id',p.id,'action_ref',p.action_ref,'run_id',p.run_id,'status',p.status,'preview',p.preview,'execution_id',e.id,'execution_status',e.status,'result_resource_id',p.result_resource_id,'audit',(SELECT coalesce(json_agg(json_build_object('action',a.action,'details',a.details)), '[]') FROM audit_events a WHERE a.enterprise_id=p.enterprise_id AND a.details->>'pending_action_id'=p.id::text)) FROM pending_actions p LEFT JOIN executions e ON e.pending_action_id=p.id WHERE p.action_ref='"+action+"';")
	if err != nil {
		return err
	}
	report := map[string]any{"task": task, "before": json.RawMessage(before), "after": json.RawMessage(after), "action": json.RawMessage(strings.TrimSpace(fact))}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-business-"+task+".json"), raw); err != nil {
		return err
	}
	var actionFact struct {
		Status          string  `json:"status"`
		ExecutionStatus string  `json:"execution_status"`
		ExecutionID     *string `json:"execution_id"`
		RunID           string  `json:"run_id"`
		Audit           []struct {
			Action  string         `json:"action"`
			Details map[string]any `json:"details"`
		} `json:"audit"`
	}
	if json.Unmarshal([]byte(fact), &actionFact) != nil {
		return fmt.Errorf("invalid action evidence")
	}
	if task == "cancel" {
		if before != after || actionFact.Status != "cancelled" || actionFact.ExecutionID != nil {
			return fmt.Errorf("model cancellation changed alert business state")
		}
		return verifyPlan6ModelAudit(actionFact.RunID, "pending_action.cancel", actionFact.Audit, false)
	}
	if actionFact.ExecutionStatus != "succeeded" {
		return fmt.Errorf("model task %s did not deterministically commit", task)
	}
	if err = verifyPlan6ModelAudit(actionFact.RunID, "pending_action.confirm", actionFact.Audit, true); err != nil {
		return err
	}
	if err = verifyPlan6UnchangedConfigurations(task, before, after); err != nil {
		return err
	}
	var configs []struct {
		ID, Kind, Name, Status string
		Spec                   struct {
			Enabled         bool      `json:"enabled"`
			Metric          string    `json:"metric"`
			Mode            string    `json:"mode"`
			Provider        string    `json:"provider"`
			IntervalSeconds int       `json:"interval_seconds"`
			ForSeconds      int       `json:"for_seconds"`
			RecoverySeconds int       `json:"recovery_seconds"`
			NoDataBehavior  string    `json:"no_data_behavior"`
			ErrorBehavior   string    `json:"error_behavior"`
			ResourceIDs     []string  `json:"resource_ids"`
			StartsAt        time.Time `json:"starts_at"`
			EndsAt          time.Time `json:"ends_at"`
			Reason          string    `json:"reason"`
			Resources       struct {
				Mode       string `json:"mode"`
				IDs, Types []string
			} `json:"resources"`
			Rules []struct {
				Expression               string `json:"expression"`
				Threshold, Recovery      float64
				RecoveryThreshold        float64 `json:"recovery_threshold"`
				Severity, Operator, Unit string
			} `json:"rules"`
			ChannelIDs []string `json:"channel_ids"`
		} `json:"spec"`
	}
	if err = json.Unmarshal([]byte(after), &configs); err != nil {
		return err
	}
	policyFound, silenceFound := false, false
	feishuID := ""
	for _, c := range configs {
		if c.Kind == "channel" && c.Name == "Plan6 feishu" && c.Spec.Provider == "feishu" {
			feishuID = c.ID
		}
	}
	for _, c := range configs {
		if c.Kind == "policy" && c.Name == "Plan6 模型双指标策略" && c.Status == "active" {
			policyFound = true
			if len(c.Spec.Rules) != 2 || len(c.Spec.ChannelIDs) != 1 || feishuID == "" || c.Spec.ChannelIDs[0] != feishuID {
				return fmt.Errorf("model did not retain atomic multi-rule policy and channel")
			}
			if c.Spec.Mode != "independent" || c.Spec.IntervalSeconds != 10 || c.Spec.ForSeconds != 0 || c.Spec.RecoverySeconds != 0 || c.Spec.NoDataBehavior != "report" || c.Spec.ErrorBehavior != "keep" || c.Spec.Resources.Mode != "explicit" || !reflect.DeepEqual(c.Spec.Resources.IDs, []string{env.State.Values["m3_cluster_id"]}) || !reflect.DeepEqual(c.Spec.Resources.Types, []string{"kubernetes_cluster"}) {
				return fmt.Errorf("model changed requested evaluation mode, timing, health behavior or resource scope during %s", task)
			}
			cpu, memory := false, false
			for _, r := range c.Spec.Rules {
				if r.Expression == "argus_plan6_cpu_percent" {
					expected := 85.0
					if task != "create" {
						expected = 90
					}
					cpu = r.Threshold == expected && r.RecoveryThreshold == 75 && r.Severity == "critical" && r.Operator == ">" && r.Unit == "%"
				}
				if r.Expression == "argus_plan6_memory_percent" {
					memory = r.Threshold == 80 && r.RecoveryThreshold == 70 && r.Severity == "warning" && r.Operator == ">" && r.Unit == "%"
				}
			}
			if !cpu || !memory {
				return fmt.Errorf("model changed unintended rules during %s", task)
			}
			if c.Spec.Enabled != (task != "disable") {
				return fmt.Errorf("model policy state did not match %s", task)
			}
		}
		if c.Kind == "silence" && c.Name == "Plan6 模型屏蔽" && c.Status == "active" {
			silenceFound = true
			if c.Spec.Metric != "argus_plan6_cpu_percent" || c.Spec.Enabled != (task == "silence") || !reflect.DeepEqual(c.Spec.ResourceIDs, []string{env.State.Values["m3_cluster_id"]}) || c.Spec.EndsAt.Sub(c.Spec.StartsAt) != time.Hour || c.Spec.Reason != "发布验证" {
				return fmt.Errorf("model silence scope/state incorrect during %s", task)
			}
		}
	}
	if !policyFound {
		return fmt.Errorf("model policy missing after %s", task)
	}
	if (task == "silence" || task == "unsilence") && !silenceFound {
		return fmt.Errorf("model silence missing after %s", task)
	}
	return nil
}

func verifyPlan6ModelAudit(runID, expected string, records []struct {
	Action  string         `json:"action"`
	Details map[string]any `json:"details"`
}, execution bool) error {
	found, committed := false, false
	for _, r := range records {
		if r.Details["run_id"] != runID || r.Details["tool_call_id"] == nil || r.Details["pending_action_id"] == nil || r.Details["resource_ids"] == nil {
			return fmt.Errorf("model action audit lost Run/ToolCall/PendingAction/resource scope")
		}
		found = found || r.Action == expected
		committed = committed || r.Details["execution_id"] != nil
	}
	if runID == "" || !found || execution && !committed {
		return fmt.Errorf("model action audit did not record %s and its execution", expected)
	}
	return nil
}

func verifyPlan6UnchangedConfigurations(task, before, after string) error {
	var left, right []map[string]any
	if json.Unmarshal([]byte(before), &left) != nil || json.Unmarshal([]byte(after), &right) != nil {
		return fmt.Errorf("invalid model configuration evidence")
	}
	prior := map[string]map[string]any{}
	for _, row := range left {
		prior[row["id"].(string)] = row
	}
	changed := 0
	for _, row := range right {
		old, exists := prior[row["id"].(string)]
		if reflect.DeepEqual(old, row) {
			continue
		}
		changed++
		target := "Plan6 模型双指标策略"
		if task == "silence" || task == "unsilence" {
			target = "Plan6 模型屏蔽"
		}
		if row["name"] != target {
			return fmt.Errorf("model %s changed an unrelated configuration", task)
		}
		if task == "create" || task == "silence" {
			if exists || row["version"] != float64(1) {
				return fmt.Errorf("model creation replaced existing configuration")
			}
			continue
		}
		if !exists {
			return fmt.Errorf("model update created a replacement configuration")
		}
		expected := old["spec"].(map[string]any)
		switch task {
		case "modify":
			for _, value := range expected["rules"].([]any) {
				r := value.(map[string]any)
				if r["expression"] == "argus_plan6_cpu_percent" {
					r["threshold"] = float64(90)
				}
			}
		case "disable", "unsilence":
			expected["enabled"] = false
		case "enable":
			expected["enabled"] = true
		}
		if !reflect.DeepEqual(expected, row["spec"]) || row["version"] != old["version"].(float64)+1 || row["status"] != old["status"] || row["kind"] != old["kind"] {
			return fmt.Errorf("model %s changed fields beyond the requested update", task)
		}
	}
	if changed != 1 || len(right) != len(left)+map[bool]int{true: 1, false: 0}[task == "create" || task == "silence"] {
		return fmt.Errorf("model %s did not make exactly one intended configuration change", task)
	}
	return nil
}
