package workspace

import (
	"bufio"
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/sandbox"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (service Service) BuildTools(ctx context.Context, p toolruntime.Principal) (toolruntime.Contribution, error) {
	part := toolruntime.Contribution{SandboxStatus: "not_configured"}
	if !service.Config.Enabled || !p.Allows("workspace.use") {
		return part, nil
	}
	current, err := service.Store.Queries.GetConversationWorkspace(ctx, db.GetConversationWorkspaceParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
	if err == nil {
		part.WorkspaceID = current.ID.String()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return part, err
	}
	if err := service.Kubernetes.Ready(ctx); err != nil {
		part.SandboxStatus = "unhealthy"
		return part, nil
	}
	runtime, err := service.Sandbox.SelectWorkspaceRuntime(ctx)
	if err != nil {
		part.SandboxStatus = "profile_unavailable"
		if !errors.Is(err, pgx.ErrNoRows) {
			part.SandboxStatus = "unhealthy"
		}
		return part, nil
	}
	if err := runtime.Client.Health(ctx); err != nil {
		part.SandboxStatus = "unhealthy"
		return part, nil
	}
	quota, err := service.Store.Queries.GetSandboxQuota(ctx, p.EnterpriseID)
	if err != nil {
		part.SandboxStatus = "quota_unavailable"
		return part, nil
	}
	active, err := service.Store.Queries.CountActiveSandboxSessions(ctx, p.EnterpriseID)
	if err != nil {
		return part, err
	}
	ownSlot := false
	if current, err := service.Get(ctx, p); err == nil && current.ActiveSandboxID.Valid {
		if sessions, err := service.Store.Queries.ListWorkspaceSandboxSessions(ctx, db.ListWorkspaceSandboxSessionsParams{WorkspaceID: uuid.NullUUID{UUID: current.ID, Valid: true}, EnterpriseID: p.EnterpriseID}); err == nil {
			for _, session := range sessions {
				ownSlot = ownSlot || session.Status == "running" && session.UpstreamSessionID == current.ActiveSandboxID.String && session.ProfileID == runtime.Profile.ID && session.ProfileRevision == runtime.Profile.Revision
			}
		}
	}
	if active >= quota.MaxConcurrentSessions && !ownSlot {
		part.SandboxStatus = "quota_unavailable"
		return part, nil
	}
	part.SandboxStatus = "ready"
	stringField := func(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }
	pathField := stringField(4096)
	schemas := map[string]map[string]any{
		"read":  toolSchema([]string{"path"}, map[string]any{"path": pathField, "start_line": map[string]any{"type": "integer", "minimum": 1}, "end_line": map[string]any{"type": "integer", "minimum": 1}}),
		"write": toolSchema([]string{"path", "content"}, map[string]any{"path": pathField, "content": stringField(1 << 20)}),
		"edit":  toolSchema([]string{"path", "old_text", "new_text"}, map[string]any{"path": pathField, "old_text": stringField(1 << 20), "new_text": stringField(1 << 20)}),
		"bash":  toolSchema([]string{"command"}, map[string]any{"command": stringField(64 << 10), "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 300}}),
	}
	identity := runtime.Identity()
	version := identity.Version()
	for _, name := range []string{"read", "write", "edit", "bash"} {
		name := name
		part.Tools = append(part.Tools, toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: name, Schema: schemas[name], Description: "Operate on the persistent /workspace in an offline sandbox. No network access."}, Source: "sandbox", Version: version, ReadOnly: name == "read"},
			Invoke: func(ctx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
				return service.builtin(ctx, name, call, identity)
			}})
	}
	return part, nil
}

func toolSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}

func (service Service) builtin(ctx context.Context, name string, call toolruntime.Invocation, identity sandbox.WorkspaceIdentity) (output toolruntime.Result, failure error) {
	defer func() { failure = fileError(failure) }()
	access, err := service.access(ctx, call.Principal, true, call.ID, accessScope{RunID: call.RunID, Runtime: &identity})
	if err != nil {
		return toolruntime.Result{}, err
	}
	defer access.Close()
	filePath, _ := call.Arguments["path"].(string)
	var result map[string]any
	switch name {
	case "read":
		reader, size, err := access.IO.Read(access.Context, filePath, 0, 0)
		if err != nil {
			return toolruntime.Result{}, err
		}
		defer reader.Close()
		start, end := 1, 200
		if value, ok := call.Arguments["start_line"].(float64); ok {
			start = int(value)
		}
		if value, ok := call.Arguments["end_line"].(float64); ok {
			end = int(value)
		}
		if end < start {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
		}
		var output strings.Builder
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		line := 0
		partial := false
		for scanner.Scan() {
			line++
			if line < start {
				continue
			}
			if line > end || output.Len()+len(scanner.Text()) > 64<<10 {
				partial = true
				break
			}
			output.WriteString(scanner.Text())
			output.WriteByte('\n')
		}
		if err := scanner.Err(); err != nil {
			return toolruntime.Result{}, err
		}
		result = map[string]any{"path": filePath, "content": output.String(), "byte_size": size, "start_line": start, "end_line": min(line, end), "partial": partial}
	case "write":
		content, _ := call.Arguments["content"].(string)
		info, err := access.IO.Write(access.Context, filePath, strings.NewReader(content), int64(len(content)), true)
		if err != nil {
			return toolruntime.Result{}, err
		}
		result = map[string]any{"path": info.Path, "byte_size": info.ByteSize, "content_hash": info.ContentHash}
	case "edit":
		old, _ := call.Arguments["old_text"].(string)
		replacement, _ := call.Arguments["new_text"].(string)
		info, err := access.IO.RPC.Edit(access.Context, &workspacev1.EditRequest{Authority: access.IO.Authority, Path: filePath, OldText: old, NewText: replacement})
		if err != nil {
			return toolruntime.Result{}, err
		}
		result = map[string]any{"path": info.Path, "byte_size": info.ByteSize, "content_hash": info.ContentHash}
	case "bash":
		command, _ := call.Arguments["command"].(string)
		timeout := 60
		if value, ok := call.Arguments["timeout_seconds"].(float64); ok {
			timeout = int(value)
		}
		output, err := access.Supervisor.RPC.Execute(access.Context, &workspacev1.ExecuteRequest{Authority: access.Supervisor.Authority, Command: command, TimeoutSeconds: int32(timeout)})
		if err != nil {
			access.reusable = false
			return toolruntime.Result{Unknown: true, Data: map[string]any{"error_code": "SANDBOX_COMMAND_RESULT_UNKNOWN"}}, nil
		}
		result = map[string]any{"stdout": output.Stdout, "stderr": output.Stderr, "exit_code": output.ExitCode, "partial": output.Partial}
	}
	if err := access.checkAuthorization(ctx); err != nil {
		return toolruntime.Result{}, err
	}
	if err := access.Close(); err != nil {
		return toolruntime.Result{}, err
	}
	return toolruntime.Result{Data: result, ToolID: name}, nil
}

func (service Service) RegisterTools(registry *mcp.Registry) error {
	fields := map[string]map[string]any{
		"workflow.import_result": toolSchema([]string{"result_ref", "path"}, map[string]any{"result_ref": map[string]any{"type": "string", "maxLength": 128}, "path": map[string]any{"type": "string", "maxLength": 4096}}),
		"workflow.publish_file":  toolSchema([]string{"path"}, map[string]any{"path": map[string]any{"type": "string", "maxLength": 4096}, "name": map[string]any{"type": "string", "maxLength": 255}, "media_type": map[string]any{"type": "string", "maxLength": 128}}),
	}
	for _, id := range []string{"workflow.import_result", "workflow.publish_file"} {
		id := id
		if err := registry.Register(mcp.Metadata{ID: id, Discovery: workspaceDiscovery[id], Risk: "write", Visibility: mcp.Visible, ExecutionMode: mcp.Sequential, Required: []string{"workspace.use"}, InputVersion: id + "/v1", OutputVersion: "argus.workspace_file/v1", MaxResultBytes: 64 << 10, InputSchema: fields[id],
			Execute: func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
				enterprise, e1 := uuid.Parse(call.Enterprise)
				user, e2 := uuid.Parse(call.Subject)
				runID, e3 := uuid.Parse(call.RunID)
				if e1 != nil || e2 != nil || e3 != nil {
					return mcp.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
				}
				run, err := service.Store.Queries.GetRun(ctx, db.GetRunParams{ID: runID, EnterpriseID: enterprise})
				if err != nil || run.ActorUserID != user {
					return mcp.Result{}, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
				}
				p, err := (conversation.Service{Store: service.Store}).ToolPrincipal(ctx, enterprise, user, run.ConversationID)
				if err != nil {
					return mcp.Result{}, err
				}
				filePath, _ := call.Input["path"].(string)
				if id == "workflow.publish_file" {
					name, _ := call.Input["name"].(string)
					media, _ := call.Input["media_type"].(string)
					value, err := service.Publish(ctx, p, runID, filePath, name, media)
					if err != nil {
						return mcp.Result{}, err
					}
					return mcp.Result{Structured: map[string]any{"artifact": deliveryView(value)}}, nil
				}
				ref, _ := call.Input["result_ref"].(string)
				file, err := service.ImportResult(ctx, p, runID, ref, filePath)
				if err != nil {
					return mcp.Result{}, err
				}
				return mcp.Result{Structured: map[string]any{"file": fileView(file), "source_result_ref": ref}}, nil
			}}); err != nil {
			return err
		}
	}
	return nil
}
