package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"slices"
	"strings"
)

// Dashboard tools reuse the public domain services. Models cannot supply a
// conversation, Run identity, compiled query, private object key or commit.
type DashboardTools struct {
	Service dashboard.Service
	Jobs    *dashboard.QueryJobs
}
type dashboardCall struct {
	actor     dashboard.Actor
	principal toolruntime.Principal
	run       uuid.UUID
	selection dashboardcontext.Snapshot
}

func (tools DashboardTools) scope(ctx context.Context, call mcp.Call) (dashboardCall, error) {
	var result dashboardCall
	enterprise, e := uuid.Parse(call.Enterprise)
	if e != nil {
		return result, mcp.ErrPermissionDenied
	}
	user, e := uuid.Parse(call.Subject)
	if e != nil || call.SubjectType != "user" || call.Caller != "model" {
		return result, mcp.ErrPermissionDenied
	}
	run, e := uuid.Parse(call.RunID)
	if e != nil || run == uuid.Nil {
		return result, mcp.ErrPermissionDenied
	}
	row, e := tools.Service.Store.Queries.GetRun(ctx, db.GetRunParams{ID: run, EnterpriseID: enterprise})
	if e != nil || row.ActorUserID != user || row.Status == "cancelled" {
		return result, mcp.ErrPermissionDenied
	}
	p, e := (conversation.Service{Store: tools.Service.Store}).ToolPrincipal(ctx, enterprise, user, row.ConversationID)
	if e != nil {
		return result, e
	}
	selection, e := dashboardcontext.ForRun(ctx, tools.Service.Store.Queries, p, run)
	if e != nil {
		return result, e
	}
	if !dashboardcontext.AllowsBusiness(selection, call.ToolID) {
		return result, mcp.ErrPermissionDenied
	}
	return dashboardCall{actor: dashboard.Actor{RunID: run, EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: p.AuthorizationVersion}, principal: p, run: run, selection: selection}, nil
}
func (c dashboardCall) selected(id uuid.UUID) error {
	if id == uuid.Nil || !slices.Contains(c.selection.DashboardIDs, id) {
		return toolruntime.Error{Kind: "DASHBOARD_SELECTION_REQUIRED", Message: "Select this dashboard in Chat before querying or editing it."}
	}
	return nil
}
func dashboardToolResult(value any, err error) (mcp.Result, error) {
	if err != nil {
		return mcp.Result{}, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return mcp.Result{}, err
	}
	var data map[string]any
	err = json.Unmarshal(raw, &data)
	return mcp.Result{Structured: data}, err
}
func dashboardToolDecode[T any](input map[string]any) (T, error) {
	var value T
	raw, err := json.Marshal(input)
	if err == nil {
		err = json.Unmarshal(raw, &value)
	}
	return value, err
}

func (tools DashboardTools) Register(registry *mcp.Registry) error {
	if tools.Service.Store == nil {
		return errors.New("dashboard store unavailable")
	}
	doc, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	type operation struct{ name, schema, id, risk, description string }
	for _, op := range []operation{
		{"list", "EmptyInput", "", "read", "列出当前用户有权查看的已发布仪表盘，供用户选择。List authorized published dashboards; never auto-select."},
		{"budget.get", "EmptyInput", "", "read", "读取当前 Run 的累计查询预算，目录、候选、统计图和下钻共用；新分页或恢复不重置。Read persistent remaining scan/result bytes, rows, samples and calls for this Run."},
		{"context.candidates", "DashboardAnalysisCandidateInput", "", "read", "读取已发布变量/局部筛选及其依赖的真实候选，不修改条件、不执行统计图。Discover candidates from a resolved context and published filter name only. Partial lists do not prove absence; candidate reconciliation is a preview until the query is queued."},
		{"get", "EmptyInput", "dashboard_id", "read", "读取用户所选仪表盘的最新发布定义和默认条件。Read selected latest published definition and defaults, without executing queries."},
		{"context.resolve", "DashboardAnalysisResolveInput", "", "read", "继承已保存条件，仅用本轮用户原话解释明确变更；无变更时 changes={} evidence=[]。Resolve inherited conditions against latest revision; cite the current user message for every change. Returns immutable context_ref; never persist inferred panel strategy or implicit defaults."},
		{"query", "DashboardAnalysisQueryInput", "", "read", "使用 context.resolve 返回的 context_ref 建立已发布查询文件任务，或沿已发布下钻继续。Query using the resolved context in this Run; no free-form parameters. Root query checks the latest revision; drilldown retains its parent scope."},
		{"query.get", "EmptyInput", "job_id", "read", "查询取数进度、文件和完整性，完成不等于已分析。Read job progress and delivered files; analyze with offline read/bash, report unchecked panels."},
		{"query.cancel", "EmptyInput", "job_id", "write", "取消本会话所选仪表盘的取数任务。Cancel a selected dashboard query job."},
		{"query.resume", "DashboardQueryResumeInput", "job_id", "write", "按版本恢复失败任务。Resume sealed file delivery or a new execution attempt; never merge different attempts."},
		{"catalog", "DashboardCatalogInput", "", "read", "创建模式下发现已接收数据的指标/字段/候选。Discover real metric metadata, fields and values; installed plugins are not proof of received data."},
		{"catalog.resources", "EmptyInput", "", "read", "列出当前有权资源及注册来源能力，再用 catalog 验证实际数据。List authorized resource names and registered source capabilities; data_state not_sampled is not evidence of received data."},
		{"draft.drilldowns", "DashboardGenerateDrilldownsInput", "draft_id", "write", "按来源和图型生成标准下钻，并按草稿版本保存。Generate standard drilldowns using the same platform definitions as the editor; publish together with the panel."},
		{"convert", "DashboardConvertPanelInput", "", "read", "使用与 UI 相同的无损构建器/语句转换。Convert through the shared server compiler; reject lossy conversions."},
		{"draft.create", "DashboardDraftInput", "", "write", "创建或恢复个人草稿，不发布；编辑已有仪表盘必须先选中它。Create a personal draft; existing dashboard opens its published baseline, then use draft.save to edit."},
		{"draft.get", "EmptyInput", "draft_id", "read", "读取当前编辑者个人草稿。Read an owned draft and its version."},
		{"draft.validate", "DashboardDraftSampleInput", "draft_id", "read", "复用人工编辑器校验草稿和样本，返回 validation.issues 的路径与原因；修正硬错误后再发布预览。Validate the owned draft version with the same compiler/runtime as the editor. valid=false is repair feedback; no_data/unavailable sample states do not mean verified healthy data."},
		{"draft.save", "DashboardDraftInput", "draft_id", "write", "按草稿版本保存修改，不弹发布确认。Save draft with expected_version; never silently overwrite conflicts."},
		{"publish.preview", "DashboardVersionInput", "draft_id", "write", "统一校验草稿并生成宿主发布确认。Validate the draft and create host confirmation. No data is a warning, not verified data health; model cannot confirm."},
	} {
		schema, e := previewSchema(doc, op.schema, op.id)
		if e != nil {
			return e
		}
		if op.name == "list" {
			schema = object([]string{}, map[string]any{"search": map[string]any{"type": "string", "maxLength": 256}, "offset": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50}})
		}
		if op.name == "catalog.resources" {
			schema = object([]string{}, map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})
		}
		name := op.name
		required := []string{"telemetry.dashboard.read"}
		if strings.HasPrefix(name, "draft.") || name == "publish.preview" {
			required = append(required, "telemetry.dashboard.manage")
		}
		if strings.HasPrefix(name, "query") {
			required = append(required, "workspace.use")
		}
		metadata := mcp.Metadata{ID: "telemetry.dashboard." + name, Risk: op.risk, Visibility: mcp.Visible, ExecutionMode: mcp.Sequential, Required: required, InputVersion: "argus.dashboard." + name + "/v1", OutputVersion: "argus.dashboard." + name + "/v1", ProjectionSchema: "argus.tool_result_projection/v1", MaxResultBytes: 2 << 20, InputSchema: schema,
			Discovery: mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: "Dashboard " + name, Description: op.description, Keywords: []string{"仪表盘", "dashboard", name}, ResultDescription: op.description, Preconditions: []string{"User-owned active conversation and immutable Run context; current object permissions are rechecked."}, Examples: []mcp.DiscoveryExample{{Request: op.description, Guidance: "Use IDs from structured user selection or preceding tool results. Never invent IDs, queries or confirmation."}}},
			Execute: func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
				value, err := tools.execute(ctx, call, name)
				return value, dashboardToolFailure(err)
			},
		}
		if name == "context.resolve" {
			metadata.Discovery.Keywords = append(metadata.Discovery.Keywords, "条件", "追问", "继承", "参数")
		}
		if name == "budget.get" {
			metadata.Discovery.Keywords = append(metadata.Discovery.Keywords, "预算", "额度", "budget")
		}
		if name == "query" {
			metadata.InputVersion = "argus.dashboard.query/v2"
			metadata.Discovery.Revision = "2"
		}
		if name == "get" {
			metadata.OutputVersion = "argus.dashboard.get/v2"
			metadata.Discovery.Revision = "2"
		}
		if name == "catalog.resources" {
			metadata.OutputVersion = "argus.dashboard.catalog.resources/v2"
			metadata.Discovery.Revision = "2"
			metadata.Discovery.Description += " Returns server_time, enterprise timezone and suggested_catalog_range (the default recent-hour from/to). Use these authoritative clock values for catalog lookup; never guess today's date from training knowledge."
		}
		if strings.HasPrefix(name, "query") {
			metadata.OutputVersion = "argus.dashboard." + name + "/v2"
			metadata.Discovery.Revision = "3"
			metadata.Discovery.Description += " The response includes the current run_budget and observation time. Failed acquisition can consume its reserved budget; distinguish the failure cause from remaining capacity. Read budget.get after file analysis before claiming capacity remains."
		}
		if name == "publish.preview" {
			metadata.OutputVersion = "argus.pending_action/v1"
			metadata.ProjectionSchema = "argus.pending_action_public/v1"
		}
		if err = registry.Register(metadata); err != nil {
			return err
		}
	}
	if err = registry.Register(mcp.Metadata{ID: "telemetry.dashboard.publish.commit", Visibility: mcp.Hidden, Risk: "dangerous", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		return mcp.Result{}, errors.New("commit is executed only from the immutable pending action")
	}}); err != nil {
		return err
	}
	return registry.ValidatePairs()
}

type dashboardFailure struct {
	cause error
	kind  string
}

func (failure dashboardFailure) Error() string { return failure.kind }
func (failure dashboardFailure) Code() string  { return failure.kind }
func (failure dashboardFailure) Unwrap() error { return failure.cause }
func dashboardToolFailure(err error) error {
	if err == nil {
		return nil
	}
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		return err
	}
	if errors.Is(err, telemetry.ErrQueryBudget) || errors.Is(err, queryengine.ErrBudget) {
		return dashboardFailure{err, "QUERY_BUDGET_EXCEEDED"}
	}
	for _, known := range []error{dashboard.ErrInvalid, dashboard.ErrDenied, dashboard.ErrConflict, dashboard.ErrArchived, dashboard.ErrNotFound, dashboard.ErrUnavailable, dashboard.ErrSelectionStale, dashboard.ErrContextExpired} {
		if errors.Is(err, known) {
			return dashboardFailure{err, known.Error()}
		}
	}
	return err
}

func (tools DashboardTools) execute(ctx context.Context, call mcp.Call, name string) (mcp.Result, error) {
	c, err := tools.scope(ctx, call)
	if err != nil {
		return mcp.Result{}, err
	}
	create := strings.HasPrefix(name, "draft.") || name == "publish.preview" || name == "catalog" || name == "convert"
	if create && c.selection.Mode != "create" {
		return mcp.Result{}, mcp.ErrPermissionDenied
	}
	switch name {
	case "budget.get":
		value, e := tools.Service.RunBudget(ctx, c.actor)
		return dashboardToolResult(value, e)
	case "list":
		rows, e := tools.Service.List(ctx, c.actor)
		if e != nil {
			return mcp.Result{}, e
		}
		items := []map[string]any{}
		for _, row := range rows {
			if row.Lifecycle == "active" && row.ActiveRevisionID.Valid && strings.Contains(strings.ToLower(row.Name+" "+row.Description), strings.ToLower(stringValue(call.Input, "search"))) {
				items = append(items, map[string]any{"id": row.ID, "name": row.Name, "description": row.Description})
			}
		}
		// Catalog has no implicit selection side effect.
		limit := int(intValue(call.Input, "limit"))
		if limit == 0 {
			limit = 20
		}
		limit = min(50, limit)
		offset := max(0, int(intValue(call.Input, "offset")))
		offset = min(offset, len(items))
		end := min(len(items), offset+limit)
		value := map[string]any{"items": items[offset:end], "has_more": end < len(items), "next_offset": end, "selection_required": len(c.selection.DashboardIDs) == 0}
		return dashboardToolResult(value, tools.Service.ChargeCatalog(ctx, c.actor, value, int64(end-offset)))
	case "get":
		id, e := callID(call, "dashboard_id")
		if e != nil {
			return mcp.Result{}, e
		}
		if e = c.selected(id); e != nil {
			return mcp.Result{}, e
		}
		item, rev, e := tools.Service.Get(ctx, c.actor, id)
		if e != nil {
			return mcp.Result{}, e
		}
		value := map[string]any{"id": item.ID, "name": item.Name, "revision_id": rev.ID, "revision_number": rev.RevisionNumber, "spec": json.RawMessage(rev.Spec)}
		if c.selection.Mode == "analyze" {
			analysis, e := tools.Service.InspectAnalysis(ctx, c.actor, c.principal.ConversationID, id)
			if e != nil {
				return mcp.Result{}, e
			}
			value["analysis"] = analysis
		}
		return dashboardToolResult(value, tools.Service.ChargeCatalog(ctx, c.actor, value, 1))
	case "context.resolve":
		input, e := dashboardToolDecode[dashboard.AnalysisResolveInput](call.Input)
		if e != nil {
			return mcp.Result{}, e
		}
		invocation, e := uuid.Parse(call.InvocationID)
		if e != nil {
			return mcp.Result{}, mcp.ErrInputInvalid
		}
		value, e := tools.Service.ResolveAnalysis(ctx, c.actor, c.principal.ConversationID, input, invocation)
		return dashboardToolResult(value, e)
	case "context.candidates":
		if tools.Jobs == nil {
			return mcp.Result{}, dashboard.ErrUnavailable
		}
		input, e := dashboardToolDecode[dashboard.AnalysisCandidateInput](call.Input)
		if e != nil {
			return mcp.Result{}, e
		}
		value, e := tools.Jobs.Runtime.AnalysisCandidates(ctx, c.actor, c.principal.ConversationID, input)
		return dashboardToolResult(value, e)
	case "query":
		if tools.Jobs == nil {
			return mcp.Result{}, dashboard.ErrUnavailable
		}
		input, e := dashboardToolDecode[dashboard.AnalysisQueryInput](call.Input)
		if e != nil {
			return mcp.Result{}, e
		}
		if e = c.selected(input.DashboardID); e != nil {
			return mcp.Result{}, e
		}
		value, e := tools.Jobs.StartAnalysis(ctx, c.actor, c.principal.ConversationID, input, idempotency(call))
		return tools.queryResult(ctx, c, value, e)
	case "query.get", "query.cancel", "query.resume":
		if tools.Jobs == nil {
			return mcp.Result{}, dashboard.ErrUnavailable
		}
		id, e := callID(call, "job_id")
		if e != nil {
			return mcp.Result{}, e
		}
		job, e := tools.Jobs.Runtime.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: id, EnterpriseID: c.actor.EnterpriseID})
		if e != nil || job.OwnerUserID != c.actor.SubjectID || job.ConversationID != c.principal.ConversationID {
			return mcp.Result{}, mcp.ErrPermissionDenied
		}
		if e = c.selected(job.DashboardID); e != nil {
			return mcp.Result{}, e
		}
		if name == "query.cancel" {
			value, e := tools.Jobs.Cancel(ctx, c.actor, c.principal.ConversationID, id)
			return tools.queryResult(ctx, c, value, e)
		}
		if name == "query.resume" {
			// A new Run must query the latest publication rather than revive old data
			// acquisition. Previously materialized files may still be delivered.
			if (!job.RunID.Valid || job.RunID.UUID != c.run) && len(job.Manifest) == 0 {
				return mcp.Result{}, dashboard.ErrContextExpired
			}
			value, e := tools.Jobs.Resume(ctx, c.actor, c.principal.ConversationID, id, intValue(call.Input, "expected_version"))
			return tools.queryResult(ctx, c, value, e)
		}
		value, e := tools.Jobs.Get(ctx, c.actor, c.principal.ConversationID, id)
		return tools.queryResult(ctx, c, value, e)
	case "catalog":
		if tools.Jobs == nil {
			return mcp.Result{}, dashboard.ErrUnavailable
		}
		input, e := dashboardToolDecode[dashboard.CatalogInput](call.Input)
		if e != nil {
			return mcp.Result{}, e
		}
		value, e := tools.Jobs.Runtime.Catalog(ctx, c.actor, input)
		return dashboardToolResult(value, e)
	case "catalog.resources":
		limit := int(intValue(call.Input, "limit"))
		if limit == 0 {
			limit = 50
		}
		value, e := tools.Service.CreationCatalog(ctx, c.actor, int(intValue(call.Input, "offset")), limit)
		if e == nil {
			e = tools.Service.ChargeCatalog(ctx, c.actor, value, int64(len(value["resources"].([]map[string]any))))
		}
		return dashboardToolResult(value, e)
	case "convert":
		input, e := dashboardToolDecode[dashboard.ConvertPanelInput](call.Input)
		if e != nil {
			return mcp.Result{}, e
		}
		return dashboardToolResult(dashboard.ConvertPanel(input), nil)
	case "draft.create":
		input, e := dashboardToolDecode[dashboard.DraftInput](call.Input)
		if e != nil {
			return mcp.Result{}, e
		}
		if input.DashboardID != uuid.Nil {
			if e = c.selected(input.DashboardID); e != nil {
				return mcp.Result{}, e
			}
		}
		invocation, e := uuid.Parse(call.InvocationID)
		if e != nil || invocation == uuid.Nil {
			return mcp.Result{}, mcp.ErrInputInvalid
		}
		value, e := tools.Service.CreateInvocationDraft(ctx, c.actor, input, invocation)
		return dashboardToolResult(toolDraft(value), e)
	default:
		id, e := callID(call, "draft_id")
		if e != nil {
			return mcp.Result{}, e
		}
		draft, e := tools.Service.Draft(ctx, c.actor, id)
		if e != nil {
			return mcp.Result{}, e
		}
		if draft.DashboardID.Valid {
			if e = c.selected(draft.DashboardID.UUID); e != nil {
				return mcp.Result{}, e
			}
		}
		if name == "draft.get" {
			return dashboardToolResult(toolDraft(draft), nil)
		}
		if name == "draft.validate" {
			if tools.Jobs == nil {
				return mcp.Result{}, dashboard.ErrUnavailable
			}
			input, e := dashboardToolDecode[struct {
				Parameters dashboard.ExecutionInput `json:"parameters"`
			}](call.Input)
			if e != nil {
				return mcp.Result{}, e
			}
			value, e := tools.Jobs.Runtime.SampleDraft(ctx, c.actor, id, intValue(call.Input, "expected_version"), input.Parameters)
			return dashboardToolResult(value, e)
		}
		if name == "draft.drilldowns" {
			input, e := dashboardToolDecode[dashboard.GenerateDrilldownsInput](call.Input)
			if e != nil {
				return mcp.Result{}, e
			}
			value, e := tools.Service.GenerateDrilldowns(ctx, c.actor, id, input)
			return dashboardToolResult(map[string]any{"draft": toolDraft(value.Draft), "added": value.Added, "issues": value.Issues}, e)
		}
		if name == "draft.save" {
			input, e := dashboardToolDecode[dashboard.DraftInput](call.Input)
			if e != nil {
				return mcp.Result{}, e
			}
			value, e := tools.Service.SaveDraft(ctx, c.actor, id, input)
			return dashboardToolResult(toolDraft(value), e)
		}
		preview, e := tools.Service.PreviewPublish(ctx, c.actor, id, intValue(call.Input, "expected_version"), idempotency(call))
		if e != nil {
			return mcp.Result{}, e
		}
		return mcp.Result{Structured: pendingProjection(preview.Action)}, nil
	}
}
func toolDraft(d db.DashboardDraft) map[string]any {
	result := map[string]any{"id": d.ID, "name": d.Name, "description": d.Description, "spec": json.RawMessage(d.Spec), "proposed_bindings": json.RawMessage(d.ProposedBindings), "draft_version": d.DraftVersion, "status": d.Status, "base_object_version": d.BaseObjectVersion}
	if spec, err := dashboard.DecodeSpec(d.Spec); err == nil {
		result["validation"] = dashboard.Validate(spec)
	}
	if d.DashboardID.Valid {
		result["dashboard_id"] = d.DashboardID.UUID
	}
	if d.BaseRevisionID.Valid {
		result["base_revision_id"] = d.BaseRevisionID.UUID
	}
	if d.FolderID.Valid {
		result["folder_id"] = d.FolderID.UUID
	}
	return result
}
