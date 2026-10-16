package argusdev

import (
	"encoding/json"
	"testing"
)

func TestPlan6PreviewRejectsUnrequestedDescriptionBeforeConfirmation(t *testing.T) {
	spec := map[string]any{"description": "CPU > 85", "enabled": true, "rules": []any{map[string]any{"expression": "argus_plan6_cpu_percent", "threshold": 85}, map[string]any{"expression": "argus_plan6_memory_percent", "threshold": 80}}}
	before, _ := json.Marshal([]any{map[string]any{"id": "policy", "kind": "policy", "name": "Plan6 模型双指标策略", "version": 1, "status": "active", "spec": spec}})
	spec["rules"].([]any)[0].(map[string]any)["threshold"] = 90
	preview := map[string]any{"id": "policy", "name": "Plan6 模型双指标策略", "spec": spec}
	raw, _ := json.Marshal(preview)
	if err := verifyPlan6PreviewUpdate("modify", string(before), string(raw)); err != nil {
		t.Fatal(err)
	}
	spec["description"] = "CPU > 90"
	raw, _ = json.Marshal(preview)
	if err := verifyPlan6PreviewUpdate("modify", string(before), string(raw)); err == nil {
		t.Fatal("unrequested description rewrite reached host confirmation")
	}
}

func TestPlan6CreatePreviewRequiresRequestedPercentUnits(t *testing.T) {
	preview := map[string]any{"spec": map[string]any{"rules": []any{
		map[string]any{"expression": "argus_plan6_cpu_percent", "operator": ">", "severity": "critical", "unit": "%", "threshold": 85, "recovery_threshold": 75},
		map[string]any{"expression": "argus_plan6_memory_percent", "operator": ">", "severity": "warning", "unit": "%", "threshold": 80, "recovery_threshold": 70},
	}}}
	raw, _ := json.Marshal(preview)
	if err := verifyPlan6CreateRulePreview(string(raw)); err != nil {
		t.Fatal(err)
	}
	preview["spec"].(map[string]any)["rules"].([]any)[0].(map[string]any)["unit"] = ""
	raw, _ = json.Marshal(preview)
	if err := verifyPlan6CreateRulePreview(string(raw)); err == nil {
		t.Fatal("missing requested unit reached host confirmation")
	}
}
