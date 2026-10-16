package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// An acceptance harness must never confirm a preview whose frozen scope fails
// the requested business intent. This is separate from post-commit evidence.
func (a *App) verifyPlan6ModelPreview(ctx context.Context, env *E2EEnvironment, task, ref, before string) error {
	if !strings.HasPrefix(ref, "act_") || strings.ContainsAny(ref, "'\\; ") {
		return fmt.Errorf("invalid owned preview")
	}
	raw, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT preview FROM pending_actions WHERE action_ref='%s' AND enterprise_id='%s';", ref, env.State.Values["enterprise_id"]))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-model-preview-"+task+".json"), []byte(strings.TrimSpace(raw))); err != nil {
		return err
	}
	if err = verifyPlan6PreviewUpdate(task, before, raw); err != nil {
		return fmt.Errorf("%w; confirmation was not sent", err)
	}
	if task == "create" {
		if err = verifyPlan6CreateRulePreview(raw); err != nil {
			return fmt.Errorf("%w; confirmation was not sent", err)
		}
	}
	var preview struct {
		ResourceIDs []string `json:"resource_ids"`
		Spec        struct {
			ResourceIDs []string `json:"resource_ids"`
			Metric      string   `json:"metric"`
		} `json:"spec"`
	}
	if json.Unmarshal([]byte(raw), &preview) != nil {
		return fmt.Errorf("invalid preview facts")
	}
	cluster := env.State.Values["m3_cluster_id"]
	if len(preview.ResourceIDs) != 1 || preview.ResourceIDs[0] != cluster {
		return fmt.Errorf("model %s preview broadened or omitted the named resource; confirmation was not sent", task)
	}
	if task == "silence" || task == "cancel" {
		metric := "argus_plan6_cpu_percent"
		if task == "cancel" {
			metric = "argus_plan6_memory_percent"
		}
		if len(preview.Spec.ResourceIDs) != 1 || preview.Spec.ResourceIDs[0] != cluster || preview.Spec.Metric != metric {
			return fmt.Errorf("model %s omitted explicit resource/metric in preview; confirmation was not sent", task)
		}
	}
	return nil
}

func verifyPlan6CreateRulePreview(raw string) error {
	var preview struct {
		Spec struct {
			Rules []struct {
				Expression, Operator, Severity, Unit string
				Threshold                            float64
				Recovery                             float64 `json:"recovery_threshold"`
			} `json:"rules"`
		} `json:"spec"`
	}
	if json.Unmarshal([]byte(raw), &preview) != nil {
		return fmt.Errorf("invalid create rule preview")
	}
	if len(preview.Spec.Rules) != 2 {
		return fmt.Errorf("model create omitted atomic two-rule preview")
	}
	cpu, memory := false, false
	for _, rule := range preview.Spec.Rules {
		if rule.Expression == "argus_plan6_cpu_percent" {
			cpu = rule.Operator == ">" && rule.Unit == "%" && rule.Severity == "critical" && rule.Threshold == 85 && rule.Recovery == 75
		}
		if rule.Expression == "argus_plan6_memory_percent" {
			memory = rule.Operator == ">" && rule.Unit == "%" && rule.Severity == "warning" && rule.Threshold == 80 && rule.Recovery == 70
		}
	}
	if !cpu || !memory {
		return fmt.Errorf("model create preview changed requested rules, units or recovery")
	}
	return nil
}

// Reuse the same exact-field invariant before confirmation and after commit.
// This projects the immutable preview into a proposed configuration fact,
// without writing business state or manufacturing execution evidence.
func verifyPlan6PreviewUpdate(task, before, raw string) error {
	if task != "modify" && task != "enable" && task != "disable" {
		return nil
	}
	var rows []map[string]any
	var preview map[string]any
	if json.Unmarshal([]byte(before), &rows) != nil || json.Unmarshal([]byte(raw), &preview) != nil {
		return fmt.Errorf("invalid preview update facts")
	}
	found := false
	for _, row := range rows {
		if row["id"] != preview["id"] {
			continue
		}
		version, ok := row["version"].(float64)
		if !ok {
			return fmt.Errorf("invalid configuration version")
		}
		row["spec"], row["name"], row["version"] = preview["spec"], preview["name"], version+1
		found = true
	}
	if !found {
		return fmt.Errorf("preview update targeted an unknown configuration")
	}
	proposed, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return verifyPlan6UnchangedConfigurations(task, before, string(proposed))
}
