package presentation

import (
	"encoding/json"
	"fmt"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"strings"
)

func ToolDisplayFor(tool string, result toolruntime.Result) *toolruntime.ToolDisplay {
	binding, err := ToolBinding(tool)
	if err != nil {
		return nil
	}
	display := &toolruntime.ToolDisplay{Mode: binding.Mode}
	data := PublicSnapshot(result.Data)
	if result.Presentation != nil {
		data = result.Presentation.Data
		bound := result.Presentation.Binding
		if bound["kind"] == "dashboard_query" {
			job, _ := bound["job_id"].(string)
			attempt, _ := bound["attempt_id"].(string)
			if job != "" && attempt != "" {
				display.ResultKey = "dashboard:" + job + ":" + attempt
			}
		}
	}
	add := func(key string, value any) {
		text := scalar(value)
		if text != "" {
			if len([]rune(text)) > 160 {
				text = string([]rune(text)[:160]) + "…"
			}
			display.Facts = append(display.Facts, toolruntime.DisplayFact{Key: key, Value: text})
		}
	}
	if binding.Entry == "change-preview" || binding.Mode == "action" {
		display.Mode = "action"
		display.ActionRef = result.ActionRef
		if display.ActionRef == "" && binding.Mode == "action" {
			display.ActionRef, _ = result.Data["action_ref"].(string)
		}
	}
	for _, key := range []string{"name", "title", "status", "from", "to"} {
		add(key, data[key])
	}
	for _, key := range []string{"host", "cluster", "connector", "collector", "dashboard", "draft"} {
		if item, ok := data[key].(map[string]any); ok {
			add("name", item["name"])
			add("status", item["status"])
		}
	}
	names := []string{}
	if rows, ok := data["resources"].([]any); ok {
		for _, raw := range rows {
			if row, ok := raw.(map[string]any); ok {
				if name, ok := row["name"].(string); ok && name != "" {
					names = append(names, name)
				}
			}
			if len(names) == 3 {
				break
			}
		}
	}
	if len(names) > 0 {
		add("resources", strings.Join(names, " · "))
	}
	countData := PublicSnapshot(result.Data)
	if result.Presentation != nil {
		if full, ok := result.Presentation.Binding["data"].(map[string]any); ok {
			countData = full
		}
	}
	if raw, ok := countData["items"]; ok {
		encoded, _ := json.Marshal(raw)
		var items []any
		if json.Unmarshal(encoded, &items) == nil {
			add("returned", fmt.Sprint(len(items)))
		}
	}
	if result.Partial {
		add("partial", "true")
	}
	if tool == "telemetry.dashboard.draft.validate" {
		validation, _ := data["validation"].(map[string]any)
		issues, _ := validation["issues"].([]any)
		if issues == nil {
			issues, _ = data["issues"].([]any)
		}
		add("validation_issues", fmt.Sprint(len(issues)))
		for _, raw := range issues {
			if row, ok := raw.(map[string]any); ok {
				display.Issues = append(display.Issues, map[string]any{"path": row["path"], "message": row["message"], "severity": row["severity"]})
			}
			if len(display.Issues) == 100 {
				break
			}
		}
		add("sample_status", data["sample_status"])
	}
	if len(display.Facts) > 8 {
		display.Facts = display.Facts[:8]
	}
	return display
}
