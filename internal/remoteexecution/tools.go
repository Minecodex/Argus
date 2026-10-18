package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func object(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}
func CommandBindingSchema() map[string]any {
	step := InputSchema(true)["properties"].(map[string]any)["steps"].(map[string]any)["items"].(map[string]any)
	props := step["properties"].(map[string]any)
	delete(props, "script")
	props["block_id"] = map[string]any{"type": "string", "pattern": blockIdentifier.String(), "description": "ID after argus: in the matching top-level Markdown fence, e.g. bash argus:backup"}
	step["required"] = []string{"block_id", "description", "target", "interpreter"}
	return step
}
func ConnectSchema() map[string]any {
	schema := InputSchema(false)
	props := schema["properties"].(map[string]any)
	props["approved_step_id"] = map[string]any{"type": "string", "format": "uuid", "description": "Use alone to execute one exact, human-approved Markdown plan step. Never supply replacement scripts or targets with this reference. Duplicate calls return its existing execution; they do not execute twice."}
	delete(schema, "required")
	schema["oneOf"] = []any{map[string]any{"required": []string{"approved_step_id"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"steps"}}, map[string]any{"required": []string{"title"}}, map[string]any{"required": []string{"description"}}, map[string]any{"required": []string{"supersedes_id"}}, map[string]any{"required": []string{"recovery_of_id"}}}}}, map[string]any{"required": []string{"title", "description", "steps"}, "not": map[string]any{"required": []string{"approved_step_id"}}}}
	return schema
}
func InputSchema(plan bool) map[string]any {
	target := object([]string{"resource_type", "resource_id"}, map[string]any{"resource_type": map[string]any{"type": "string", "enum": []string{"host", "kubernetes_cluster"}}, "resource_id": map[string]any{"type": "string", "format": "uuid"}, "namespace": map[string]any{"type": "string", "maxLength": 253}, "pod": map[string]any{"type": "string", "maxLength": 253}, "pod_uid": map[string]any{"type": "string", "maxLength": 128}, "container": map[string]any{"type": "string", "maxLength": 253}})
	step := object([]string{"description", "target", "interpreter", "script"}, map[string]any{"description": map[string]any{"type": "string", "maxLength": 4096}, "target": target, "interpreter": map[string]any{"type": "string", "enum": []string{"bash", "powershell", "kubectl", "pod_bash"}}, "script": map[string]any{"type": "string", "minLength": 1, "maxLength": MaxScriptBytes, "description": "Complete native commands in the shared interpreter. The platform captures exit status. Do not append exit only to report status; exit/exec or persistent errexit can end shared context and require a newly confirmed recovery. For expected diagnostic failures capture the status and continue."}, "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 1800}})
	maxSteps := 1
	if plan {
		maxSteps = MaxSteps
	}
	return object([]string{"title", "description", "steps"}, map[string]any{
		"title":          map[string]any{"type": "string", "minLength": 1, "maxLength": 256, "description": "Required short title. Include title in the arguments object, not only in narration."},
		"description":    map[string]any{"type": "string", "maxLength": 16384, "description": "Required purpose and evidence for this request."},
		"steps":          map[string]any{"type": "array", "minItems": 1, "maxItems": maxSteps, "items": step, "description": "Ordered, separately described operations. Related commands may share one script; all steps in a fixed plan run serially."},
		"supersedes_id":  map[string]any{"type": "string", "format": "uuid", "description": "ID of the earlier unexecuted plan being revised, not an action_ref or resource ID."},
		"recovery_of_id": map[string]any{"type": "string", "format": "uuid", "description": "Earlier execution.id whose interpreter was lost or reclaimed. Required to prepare its fresh context. Read durable facts first; never blindly repeat successful or unknown writes. A revised recovery may retain this ID and also set supersedes_id."},
	})
}
func (s *Service) BuildTools(ctx context.Context, p toolruntime.Principal) (toolruntime.Contribution, error) {
	if !p.Allows("remote_execution.execute") {
		return toolruntime.Contribution{}, nil
	}
	tool := toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: "connect", Description: "Prepare one complete remote command batch. The backend applies its captured approval policy: request_approval waits for the real user; full_access confirms new batches automatically, including evidence collection in plan mode. Use a resolved authorized resource UUID and its native interpreter. Place input files in Workspace remote/input; the platform freezes and uploads them after confirmation. Write return files under ARGUS_OUTPUT_DIR (PowerShell: $env:ARGUS_OUTPUT_DIR). ARGUS_INPUT_DIR contains uploaded inputs. Matching resources in this conversation share directory, variables and functions across approved batches; every new batch follows current access_mode. Use workflow plan.propose for Markdown plans. After human plan confirmation, call connect with only approved_step_id from approved_plan.commands to consume a frozen command once. Do not copy or modify its script. New commands use normal arguments and the current approval policy. Unknown or lost sessions require analysis of durable facts; supply recovery_of_id for a fresh context and never replay completed or unknown changes." + toolruntime.RemoteFileScopeInstructions, Schema: ConnectSchema()}, Source: "argus", Scenario: "remote", Version: "2", ReadOnly: true}, Invoke: func(ctx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
		if ref, ok := call.Arguments["approved_step_id"]; ok {
			text, valid := ref.(string)
			id, e := uuid.Parse(text)
			if !valid || e != nil || len(call.Arguments) != 1 {
				return toolruntime.Result{}, Error("REMOTE_INPUT_INVALID")
			}
			view, e := s.PrepareApproved(ctx, call, id)
			if e != nil {
				return toolruntime.Result{}, e
			}
			return connectResult(view), nil
		}
		var input Input
		b, _ := json.Marshal(call.Arguments)
		if json.Unmarshal(b, &input) != nil {
			return toolruntime.Result{}, Error("REMOTE_INPUT_INVALID")
		}
		view, err := s.Prepare(ctx, call, "connect", input)
		if err != nil {
			return toolruntime.Result{}, err
		}
		return connectResult(view), nil
	}}
	return toolruntime.Contribution{Tools: []toolruntime.Tool{tool}}, nil
}

func connectResult(view View) toolruntime.Result {
	return toolruntime.Result{Data: map[string]any{"execution": view, "action_ref": view.ActionRef}, ActionRef: view.ActionRef, ToolID: "connect", Display: &toolruntime.ToolDisplay{Mode: "action", ActionRef: view.ActionRef}}
}
func (s *Service) RegisterTools(registry *mcp.Registry) error {
	// The registry declares the host-only pair; Action.CommitAction below owns
	// execution from the immutable plan, as for existing native previews.
	if err := registry.Register(mcp.Metadata{ID: "workflow.remote_plan.commit", Risk: "write", Visibility: mcp.Hidden, ExecutionMode: mcp.Sequential, Required: []string{"internal.action_executor"}, InputVersion: "argus.private_action_plan/v1", OutputVersion: "argus.execution/v1", ProjectionSchema: "none", MaxResultBytes: 1024, InputSchema: object(nil, map[string]any{}), Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		return mcp.Result{}, errors.New("commit is executed from the immutable action plan")
	}}); err != nil {
		return err
	}
	discovery := mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: "远程修复计划 / Remote repair plan", Description: "将描述和原生命令组成固定串行计划，准备一次宿主确认；支持修改意见和上下文恢复。 Prepare an immutable serial remote plan for one host confirmation, revision and recovery.", Keywords: []string{"远程", "修复", "计划", "恢复", "命令", "remote", "repair", "plan", "recovery"}, ResultDescription: "返回实际目标、账号、完整命令、输入文件清单、计划版本及待确认动作；不执行命令。 Returns an authoritative plan preview, frozen inputs and pending action, without execution.", Preconditions: []string{"先取数并在 Workspace 分析；资源有显式执行授权，命令使用目标原生解释器。 Analyze evidence first; use explicitly authorized resources and native interpreters."}, Examples: []mcp.DiscoveryExample{{Request: "排查后生成修复计划 / Prepare a repair plan after investigation", Guidance: "steps 按执行顺序包含 description、target、interpreter 和完整 script；修订传 supersedes_id，恢复传 recovery_of_id。 Provide ordered steps and complete scripts; reference the previous revision for changes or recovery."}}}
	return registry.Register(mcp.Metadata{ID: "workflow.remote_plan.preview", Discovery: discovery, Risk: "write", Visibility: mcp.Hidden, ExecutionMode: mcp.Sequential, Required: []string{"remote_execution.execute"}, InputVersion: SchemaVersion, OutputVersion: SchemaVersion, MaxResultBytes: 1 << 20, InputSchema: InputSchema(true), Execute: func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
		enterprise, e1 := uuid.Parse(call.Enterprise)
		user, e2 := uuid.Parse(call.Subject)
		runID, e3 := uuid.Parse(call.RunID)
		id, e4 := uuid.Parse(call.InvocationID)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return mcp.Result{}, Error("REMOTE_INPUT_INVALID")
		}
		run, err := s.Store.Queries.GetRun(ctx, db.GetRunParams{ID: runID, EnterpriseID: enterprise})
		if err != nil || run.ActorUserID != user {
			return mcp.Result{}, Error("REMOTE_EXECUTION_FORBIDDEN")
		}
		p, err := (conversation.Service{Store: s.Store}).ToolPrincipal(ctx, enterprise, user, run.ConversationID)
		if err != nil {
			return mcp.Result{}, err
		}
		var input Input
		b, _ := json.Marshal(call.Input)
		if json.Unmarshal(b, &input) != nil {
			return mcp.Result{}, Error("REMOTE_INPUT_INVALID")
		}
		view, err := s.Prepare(ctx, toolruntime.Invocation{Principal: p, RunID: runID, ID: id}, "plan", input)
		if err != nil {
			return mcp.Result{}, err
		}
		return mcp.Result{Structured: map[string]any{"execution": view, "action_ref": view.ActionRef}}, nil
	}})
}
