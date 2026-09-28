//go:build m4e2e

package main

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
)

type replayStep struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

var requestedTool = regexp.MustCompile(`(?i)\bcall\s+([a-z][a-z0-9_.]+)`)

// Replay instructions are test fixtures, never production tool selection.
// Every response uses the same native protocol consumed by real providers.
func selectTool(request replayRequest) (string, string, bool) {
	messages := append(request.Messages, request.Input...)
	start := -1
	prompt := ""
	for index, item := range messages {
		if item.Role == "user" {
			if text := replayContentText(item.Content); text != "" {
				start = index
				prompt = text
			}
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(prompt)), "verify this deterministic execution result") {
		return "", "", false
	}
	calls := []string{}
	outputs := 0
	for _, item := range messages[start+1:] {
		for _, call := range item.ToolCalls {
			calls = append(calls, call.Function.Name)
		}
		if item.Type == "function_call" {
			calls = append(calls, item.Name)
		}
		if item.Type == "function_call_output" || item.Role == "tool" {
			outputs++
		}
	}
	available := map[string]map[string]any{}
	for _, tool := range request.Tools {
		if tool.Function != nil {
			available[tool.Function.Name] = tool.Function.Parameters
		} else {
			available[tool.Name] = tool.Parameters
		}
	}
	marshal := func(name string, args map[string]any) (string, string, bool) {
		wire := name
		if _, ok := available[wire]; !ok {
			wire = strings.ReplaceAll(name, ".", "_")
		}
		if _, ok := available[wire]; !ok {
			return "", "", false
		}
		data, _ := json.Marshal(args)
		return wire, string(data), true
	}
	const marker = "argus_e2e_plan_b64:"
	if index := strings.Index(strings.ToLower(prompt), marker); index >= 0 {
		words := strings.Fields(prompt[index+len(marker):])
		if len(words) == 0 {
			return "", "", false
		}
		data, err := base64.RawURLEncoding.DecodeString(words[0])
		if err != nil {
			return "", "", false
		}
		var plan []replayStep
		if json.Unmarshal(data, &plan) != nil || len(calls) >= len(plan) {
			return "", "", false
		}
		step := plan[len(calls)]
		resolved, err := replayResultArguments(step.Arguments, messages[start+1:])
		if err != nil {
			return "", "", false
		}
		step.Arguments = resolved
		if strings.HasPrefix(step.Tool, "mcp:") {
			original := strings.TrimPrefix(step.Tool, "mcp:")
			for _, tool := range request.Tools {
				name, description := tool.Name, tool.Description
				if tool.Function != nil {
					name, description = tool.Function.Name, tool.Function.Description
				}
				if strings.HasPrefix(name, "mcp_") && strings.Contains(description, original) {
					return marshal(name, step.Arguments)
				}
			}
			return "", "", false
		}
		return marshal(step.Tool, step.Arguments)
	}
	if _, ok := available["compatibility_probe"]; ok {
		if outputs > 0 {
			return "", "", false
		}
		return selectedTool("compatibility_probe", available["compatibility_probe"])
	}
	matched := requestedTool.FindStringSubmatch(prompt)
	if _, gateway := available["tool_search"]; gateway && len(matched) == 2 {
		category, name := nativeIdentity(matched[1])
		if category == "" {
			return "", "", false
		}
		switch len(calls) {
		case 0:
			return marshal("tool_search", map[string]any{"category": category, "query": name})
		case 1:
			return marshal("tool_describe", map[string]any{"category": category, "name": name})
		case 2:
			schema := map[string]any{}
			for index := len(messages) - 1; index > start; index-- {
				item := messages[index]
				text, _ := item.Content.(string)
				if item.Type == "function_call_output" {
					text = item.Output
				}
				var value struct {
					Summary struct {
						InputSchema map[string]any `json:"input_schema"`
					} `json:"summary"`
				}
				if json.Unmarshal([]byte(text), &value) == nil && value.Summary.InputSchema != nil {
					schema = value.Summary.InputSchema
					break
				}
			}
			_, raw, _ := selectedToolWithPrompt(name, schema, prompt)
			var args map[string]any
			_ = json.Unmarshal([]byte(raw), &args)
			return marshal("tool_invoke", map[string]any{"category": category, "name": name, "arguments": args})
		default:
			return "", "", false
		}
	}
	if outputs > 0 || len(calls) > 0 {
		return "", "", false
	}
	if len(matched) == 2 {
		if schema, ok := available[matched[1]]; ok {
			return selectedToolWithPrompt(matched[1], schema, prompt)
		}
	}
	return "", "", false
}

func replayContentText(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	var text strings.Builder
	if parts, ok := content.([]any); ok {
		for _, part := range parts {
			value, ok := part.(map[string]any)
			if !ok || value["type"] != "input_text" && value["type"] != "output_text" {
				continue
			}
			if item, ok := value["text"].(string); ok {
				text.WriteString(item)
			}
		}
	}
	return text.String()
}

func nativeIdentity(value string) (string, string) {
	switch value {
	case "telemetry.promql.query":
		return "metric", "query"
	case "telemetry.kql.query":
		return "log", "query"
	case "telemetry.skywalking.trace":
		return "trace", "query"
	case "telemetry.overview":
		return "metric", "overview"
	case "telemetry.collector.list":
		return "connector", "collector.list"
	case "telemetry.collector.get":
		return "connector", "collector.get"
	}
	prefix, name, ok := strings.Cut(value, ".")
	if !ok {
		return "", ""
	}
	switch prefix {
	case "host", "connector", "workflow":
		return prefix, name
	case "kubernetes":
		return "k8s", name
	case "pending_action":
		return "workflow", value
	}
	return "", ""
}
