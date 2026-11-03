package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type ObservationTools struct {
	Base       ResourceTools
	Monitoring monitoring.Service
	Telemetry  telemetry.Service
	Dashboard  dashboard.Service
	CursorKey  []byte
}

func (t ObservationTools) Register(reg *mcp.Registry) error {
	doc, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	catalog, err := previewSchema(doc, "ObservationCatalogInput", "")
	if err != nil {
		return err
	}
	catalog["properties"].(map[string]any)["scope"] = telemetry.ModelScopeSchema()
	list := objectSchema(map[string]any{"resource_types": domainStrings(5), "families": domainStrings(32), "integrations": domainStrings(64), "search": domainText(200), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "cursor": domainText(4096)})
	listFields := list["properties"].(map[string]any)
	listFields["resource_types"] = telemetry.ModelScopeSchema()["properties"].(map[string]any)["resource_types"]
	families, integrations := []string{"infrastructure"}, []string{}
	familyGuide := []string{}
	for _, entry := range monitoringcatalog.All() {
		if !slices.Contains(families, entry.Family) {
			families = append(families, entry.Family)
		}
		integrations = append(integrations, entry.Key)
		familyGuide = append(familyGuide, entry.Key+"="+entry.Family)
	}
	for _, entry := range builtincollection.All() {
		integrations = append(integrations, entry.Key)
	}
	slices.Sort(families)
	slices.Sort(integrations)
	slices.Sort(familyGuide)
	listFields["families"].(map[string]any)["description"] = "Optional registry family intersection. For an exact integration, prefer integrations alone instead of adding an assumed family. Families are registered data, not inferred from a technology's common use. Registered mappings: " + strings.Join(familyGuide, ", ")
	listFields["families"].(map[string]any)["items"] = map[string]any{"type": "string", "enum": families}
	listFields["integrations"].(map[string]any)["items"] = map[string]any{"type": "string", "enum": integrations}
	refs := telemetry.ModelScopeSchema()["properties"].(map[string]any)["resources"].(map[string]any)
	refs["maxItems"] = 20
	caps := objectSchema(map[string]any{"resources": refs, "signals": map[string]any{"type": "array", "maxItems": 3, "uniqueItems": true, "items": map[string]any{"enum": []string{"metrics", "logs", "traces"}}}, "sections": map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "uniqueItems": true, "items": map[string]any{"enum": []string{"collection_state", "sources", "dashboards", "next_tools"}}}, "cursor": domainText(8192)}, "resources")
	for _, entry := range []struct {
		id, title, description string
		schema                 map[string]any
		run                    func(context.Context, authorization.Actor, mcp.Call) (any, error)
	}{
		{"observe.resources.list", "发现观测资源 / Find observation resources", "Find authorized resources by exact type, integration/family and name. Arguments are flat: use resource_types (array) and search (string) at the top level; never invent a filters object or singular resource_type/name fields. Filters intersect before paging. Follow next_cursor; partial pages do not prove absence. Use authoritative type/id refs, clarify ambiguous names, then inspect capabilities only if needed.", list, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			in, e := domainInput[observationscope.DiscoveryInput](c)
			if e != nil {
				return nil, e
			}
			return (observationscope.Resolver{Queries: t.Base.Store.Queries}).Discover(ctx, a, t.CursorKey, in)
		}},
		{"observe.capabilities.get", "观测能力与来源 / Observation capabilities and sources", "Read bounded collection states, registered sources, suitable dashboards and next tool identities for explicit resources. Registration is not observed data or health. sampling_performed=false and data_state=not_checked mean telemetry samples were not queried. Collection desired_enabled=false, health=stopped or freshness=no_data do not prove historical telemetry is absent. next_tools contains tool identity references, not a promise that every signal has configured sources or data. 停用及 freshness 状态不能证明历史数据为空；后续工具引用不等于信号已有数据。 Credentials and private model governance are excluded. For a section next_cursor, repeat with only that resource and section. Tool references still require successful search and describe.", caps, t.capabilities},
		{"observe.catalog.query", "指标与字段目录 / Metrics and fields catalog", "Discover actual metrics, fields, values and sources in an explicit authorized scope and absolute time window. Includes source-specific versioned query recipes; a recipe is not a measurement. Same-named metrics with different sources/units stay separate. observed_only defaults true; false includes supported but unobserved entries. Follow bounded cursors and preserve completeness. complete=false can describe a searched/paged subset; use has_more and next_cursor for continuation and do not equate this flag with missing telemetry. Use metric for an exact metric-name lookup.", catalog, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			in, e := domainInput[telemetry.ObservationCatalogInput](c)
			if e != nil {
				return nil, e
			}
			if in.Scope.Mode != "explicit" || len(in.Scope.Resources) == 0 {
				return nil, telemetry.ErrQueryInvalid
			}
			actor, e := t.Telemetry.ResourceQueryActor(ctx, a.EnterpriseID, a.SubjectID, a.SubjectType, a.AuthorizationVersion)
			if e != nil {
				return nil, e
			}
			actor.RunID = uuid.NullUUID{UUID: a.RunID, Valid: a.RunID != uuid.Nil}
			service := t.Telemetry
			if service.ModelQueryMeter != nil {
				service.Engine = service.ModelQueryMeter(actor, service.Engine)
			}
			return service.ObservationCatalog(ctx, actor, in, dashboard.ObservationRecipes())
		}},
	} {
		entry := entry
		compiled, e := toolruntime.CompileInputSchema(entry.schema)
		if e != nil {
			return e
		}
		meta := mcp.Metadata{ID: entry.id, Risk: "read", Visibility: mcp.Visible, ExecutionMode: mcp.ParallelSafe, AnyRequired: telemetry.ResourceQueryPermissions(), InputVersion: "argus.observe/v1", OutputVersion: "argus.observe_result/v1", MaxResultBytes: 1 << 20, InputSchema: entry.schema, OutputSchema: map[string]any{"type": "object"},
			Discovery: mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: entry.title, Description: entry.description, Keywords: []string{"observe", "resources", "metrics", "观测"}, ResultDescription: "Authorized, bounded data with actual source, time and completeness; no credentials or implicit execution.", Preconditions: []string{"Use explicit authorized resource identities. Resource discovery is not authorization for other objects or signals."}, Examples: []mcp.DiscoveryExample{{Request: entry.title, Guidance: "Read this exact schema before invocation. Use IDs and sources from current authorized facts, not inferred names."}}},
			Validate: func(in map[string]any) error {
				raw, e := json.Marshal(in)
				if e != nil {
					return e
				}
				var value any
				if json.Unmarshal(raw, &value) != nil {
					return mcp.ErrInputInvalid
				}
				return compiled.Validate(value)
			},
			Authorize: func(ctx context.Context, c mcp.Call) error {
				_, e := (MonitoringTools{Base: t.Base}).actor(ctx, c)
				return e
			},
			Execute: func(ctx context.Context, c mcp.Call) (mcp.Result, error) {
				a, e := (MonitoringTools{Base: t.Base}).actor(ctx, c)
				if e != nil {
					return mcp.Result{}, e
				}
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				value, e := t.Dashboard.ObservationRead(ctx, a, func() (any, error) { return entry.run(ctx, a, c) })
				if e != nil {
					return mcp.Result{}, e
				}
				raw, e := json.Marshal(value)
				if e != nil {
					return mcp.Result{}, e
				}
				if len(raw) > 1<<20 {
					return mcp.Result{}, fmt.Errorf("observation result budget exceeded")
				}
				out := map[string]any{}
				if e = json.Unmarshal(raw, &out); e != nil {
					return mcp.Result{}, e
				}
				out["observed_at"] = time.Now().UTC()
				out["request_context"] = presentation.PublicSnapshot(c.Input)
				if entry.id == "observe.catalog.query" {
					out["page_counts"] = observationPageCounts(out)
				}
				return mcp.Result{Structured: out, Partial: out["complete"] == false}, nil
			},
		}
		meta = t.observationPresentation(meta)
		if entry.id == "observe.catalog.query" {
			meta.OutputVersion = "argus.observe_result/v4"
			meta.Discovery.Revision = "5"
			meta.Discovery.ResultDescription = "Authorized catalog metadata, not numeric measurements. page_counts distinguishes returned metric entries/names and source IDs/versions for this page only. complete=false may describe a searched/scoped subset, not a clipped or unreadable original. Use has_more/next_cursor for continuation, or narrow discovery. Use returned observed names/source_keys/query_methods to search/describe/invoke metric.query for requested values. No measurement Workspace file exists before a data query; result_ref is an opaque identifier, not a path. Never search the container filesystem for it."
			meta.Discovery.Description += " query_methods on observed histogram/summary items identifies classic versus native representation and valid count/sum/mean expressions with result units; follow those actual methods. Classic explicit histograms use suffix _count/_sum, not native histogram_count(base). Preserve source/version, label scope, original time bounds and unit variants; methods are not measurements."
			meta.Discovery.Description += " semantics on matched observed builtin entries describes a source-qualified input quantity and recommended display; raw type/unit remain unchanged. Known utilization ratios use percent display; unit1 alone on other/unknown sources is not evidence of a ratio. Successful PromQL execution does not validate physical units or Gauge/Counter business semantics; float aggregation may execute while being semantically invalid."
		}
		if entry.id == "observe.capabilities.get" {
			meta.Discovery.Revision = "7"
			meta.OutputVersion = "argus.observe_result/v5"
			meta.Discovery.Description = "Per-resource capability navigation. Dashboard links in each item describe that ONE resource, not combined applicability or a separately requested dashboard filter. For type/signal/integration dashboard searches or partial applicability across several resources, search/describe dashboard.list and pass that explicit resource set and filters. Do not infer aggregate partial suitability from empty per-resource sections. "
			meta.Discovery.Description += "Read bounded collection states, source-version registrations, suitable dashboards and next-tool identities for explicit authorized resources; no probes, credentials or private model governance. Root sampling_performed=false/data_state=not_checked describe THIS metadata request, not target history or health; stopped/no_data never prove historical absence. collection_state covers collection instances only, not SDK endpoints or all sources: empty does not mean no monitoring. configured_collector_id/effective_collector_id are separate anchors; placement_state=requires_authorized_resolution means this response has NOT verified a Host/Cluster relation. Resolve an actual authorized Collector binding before selecting by placement; names, shared IP or an online Host are insufficient. If unresolved, ask rather than guess. sources items are version selectors, not distinct source counts: use stable source_id for identity/counting; page counts cover this page only. Preserve full source_key including revision/Namespace facet for queries. current_installation means Collector generation, not latest revision, SDK enabled, receiving or health. Requested SDK protocol/signals/enabled facts require authorized endpoint metadata. next_tools are references, not proof of sources/data, and still require Search/Describe. Continue a section next_cursor with only that resource and section."
		}
		if e = reg.Register(meta); e != nil {
			return e
		}
	}
	return nil
}

func domainStrings(max int) map[string]any {
	return map[string]any{"type": "array", "maxItems": max, "uniqueItems": true, "items": domainText(128)}
}

func (t ObservationTools) observationPresentation(meta mcp.Metadata) mcp.Metadata {
	source, _ := templates.ReadFile("templates/observation.html")
	asset := presentation.ToolAsset(meta.ID, string(source))
	meta.Template = asset
	meta.Present = func(ctx context.Context, c mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		if c.SubjectType == "service_account" {
			return nil, nil
		}
		a, e := (MonitoringTools{Base: t.Base}).actor(ctx, c)
		if e != nil {
			return nil, e
		}
		access, e := presentation.CurrentAccess(ctx, t.Base.Store, a.EnterpriseID, a.SubjectID)
		if e != nil {
			return nil, e
		}
		data := presentation.BindData(meta.ID, meta.Discovery.Title, result.Structured)
		refs := []toolruntime.ResourceRef{}
		seen := map[string]bool{}
		denied := false
		add := func(ref resourceref.Ref) {
			r := toolruntime.ResourceRef{Type: ref.Type, ID: ref.ID.String()}
			if !access.AllowsResource(r) {
				denied = true
				return
			}
			if !seen[r.Type+"/"+r.ID] {
				seen[r.Type+"/"+r.ID] = true
				refs = append(refs, r)
			}
		}
		if scope, ok := c.Input["scope"]; ok {
			raw, _ := json.Marshal(scope)
			var in telemetry.QueryScope
			if json.Unmarshal(raw, &in) == nil {
				for _, ref := range in.Resources {
					add(ref)
				}
			}
		}
		if raw, ok := c.Input["resources"]; ok {
			encoded, _ := json.Marshal(raw)
			var values []resourceref.Ref
			if json.Unmarshal(encoded, &values) == nil {
				for _, ref := range values {
					add(ref)
				}
			}
		}
		if items, ok := result.Structured["items"].([]any); ok {
			for _, raw := range items {
				if item, ok := raw.(map[string]any); ok {
					if v, ok := item["ref"]; ok {
						encoded, _ := json.Marshal(v)
						var ref resourceref.Ref
						if json.Unmarshal(encoded, &ref) == nil {
							add(ref)
						}
					}
				}
			}
		}
		if denied {
			return nil, mcp.ErrPermissionDenied
		}
		ids := []uuid.UUID{}
		for _, ref := range refs {
			ids = append(ids, uuid.MustParse(ref.ID))
		}
		names, _, err := presentation.Resources(ctx, t.Base.Store, a.EnterpriseID, ids)
		if err != nil {
			return nil, err
		}
		data["resources"] = names
		data["_resource_refs"] = refs
		return presentation.Bound(asset, data, access.Scope, refs), nil
	}
	return meta
}
