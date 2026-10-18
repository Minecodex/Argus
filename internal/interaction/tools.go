package interaction

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func obj(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}
func str(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }
func QuestionSchema() map[string]any {
	option := obj([]string{"label", "description"}, map[string]any{"label": str(256), "description": str(2048)})
	question := obj([]string{"id", "header", "question", "options"}, map[string]any{"id": map[string]any{"type": "string", "pattern": identifier.String()}, "header": str(128), "question": str(4096), "options": map[string]any{"type": "array", "maxItems": 4, "items": option, "description": "Two to four distinct choices, recommended first. Empty for free text only. The UI always accepts custom text; never add an Other option."}})
	return obj([]string{"questions"}, map[string]any{"questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": question}})
}
func PlanSchema() map[string]any {
	return obj([]string{"title", "markdown"}, map[string]any{"title": str(256), "markdown": map[string]any{"type": "string", "maxLength": 262144, "description": "Complete plan.md. Executable blocks must be top-level fenced blocks with info bash argus:step_id or powershell argus:step_id. Every executable block has one matching command_bindings entry. Normal examples are not authorized. Never include stdout or invented execution results in a plan." + toolruntime.RemoteFileScopeInstructions}, "command_bindings": map[string]any{"type": "array", "maxItems": 64, "items": remoteexecution.CommandBindingSchema()}, "supersedes_id": map[string]any{"type": "string", "format": "uuid"}})
}
func (s Service) BuildTools(ctx context.Context, p toolruntime.Principal) (toolruntime.Contribution, error) {
	if !p.Allows("conversation.use") {
		return toolruntime.Contribution{}, nil
	}
	tool := toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "request_user_input", Description: "Ask the real user one to three concise questions and wait for their response. Use when missing preferences or facts prevent progress. Supply short options with tradeoff descriptions, or an empty options array for free text. Available in execute and plan modes. Full access never answers these questions. This is not an execution approval tool.", Schema: QuestionSchema()}, Source: "argus", Scenario: "interaction", Version: "1", ReadOnly: true}, Invoke: func(ctx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
		raw, _ := json.Marshal(call.Arguments)
		var input Questions
		if json.Unmarshal(raw, &input) != nil {
			return toolruntime.Result{}, invalid()
		}
		if err := input.Validate(); err != nil {
			return toolruntime.Result{}, err
		}
		r, err := s.prepare(ctx, call, "user_input", input)
		if err != nil {
			return toolruntime.Result{}, err
		}
		if r.Status == "pending" {
			return toolruntime.Result{Control: &toolruntime.Control{InteractionID: r.ID, Kind: r.Kind}}, nil
		}
		if r.Status == "cancelled" {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_CANCELLED"}
		}
		var answer map[string]any
		_ = json.Unmarshal(r.Response, &answer)
		return toolruntime.Result{Data: map[string]any{"request_id": r.ID, "status": r.Status, "response": answer}, ToolID: "request_user_input"}, nil
	}}
	return toolruntime.Contribution{Tools: []toolruntime.Tool{tool}}, nil
}
func (s Service) RegisterTools(registry *mcp.Registry) error {
	discovery := mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: "提出实施计划 / Propose implementation plan", Description: "生成 Markdown 计划，冻结明确绑定的命令，等待用户批准后由 Harness 通过 connect 正常执行。 Propose plan.md with explicitly bound executable fences. Human confirmation authorizes those exact commands; implementation remains ordinary Harness tool calls.", Keywords: []string{"计划", "plan", "实施", "implementation"}, ResultDescription: "持久化不可变文档和命令清单；用户确认后，步骤 ID 获得一次执行权限。 Immutable document and command IDs; consent grants one execution per step.", Preconditions: []string{"先分析已有证据，必要时询问用户。 Analyze evidence and clarify missing preferences first."}, Examples: []mcp.DiscoveryExample{{Request: "生成修复计划 / Propose a repair plan", Guidance: "提供 title、markdown、command_bindings；代码围栏使用 bash argus:backup，绑定 block_id=backup、target、interpreter、description。纯说明计划绑定为空数组。 Use top-level executable fences and resolved resource UUIDs; supersedes_id identifies a revision."}}}
	return registry.Register(mcp.Metadata{ID: "workflow.plan.propose", Discovery: discovery, Risk: "read", Visibility: mcp.Visible, ExecutionMode: mcp.Sequential, Required: []string{"conversation.use"}, InputVersion: "argus.markdown_plan/v2", OutputVersion: "argus.markdown_plan/v2", MaxResultBytes: 128 << 10, InputSchema: PlanSchema(), Execute: func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
		enterprise, e1 := uuid.Parse(call.Enterprise)
		user, e2 := uuid.Parse(call.Subject)
		runID, e3 := uuid.Parse(call.RunID)
		id, e4 := uuid.Parse(call.InvocationID)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return mcp.Result{}, invalid()
		}
		run, err := s.Store.Queries.GetRun(ctx, db.GetRunParams{ID: runID, EnterpriseID: enterprise})
		if err != nil || run.ActorUserID != user {
			return mcp.Result{}, forbidden()
		}
		p, err := s.Conversations.ToolPrincipal(ctx, enterprise, user, run.ConversationID)
		if err != nil {
			return mcp.Result{}, err
		}
		raw, _ := json.Marshal(call.Input)
		var input Plan
		if json.Unmarshal(raw, &input) != nil {
			return mcp.Result{}, invalid()
		}
		if err = input.Validate(); err != nil {
			return mcp.Result{}, err
		}
		r, err := s.ProposePlan(ctx, toolruntime.Invocation{ID: id, RunID: runID, Principal: p}, input)
		if err != nil {
			return mcp.Result{}, err
		}
		return mcp.Result{Structured: map[string]any{"plan_id": r.ID, "revision": r.Revision, "status": r.Status, "summary": "Wait for human confirmation. Then invoke connect with approved_step_id from the approved plan context; do not retype or modify its scripts."}, Control: &toolruntime.Control{InteractionID: r.ID, Kind: "plan"}}, nil
	}})
}
