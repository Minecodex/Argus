package telemetry

import (
	"context"
	"embed"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

//go:embed templates/*.html
var toolTemplates embed.FS

func (tools Tools) presentationMetadata(metadata mcp.Metadata) mcp.Metadata {
	name := "overview"
	switch metadata.ID {
	case "telemetry.promql.query":
		name = "metric"
	case "telemetry.kql.query":
		name = "log"
	case "telemetry.skywalking.trace":
		name = "trace"
	}
	source, _ := toolTemplates.ReadFile("templates/" + name + ".html")
	asset := presentation.ToolAsset(metadata.ID, string(source))
	metadata.Template = asset
	metadata.Present = func(ctx context.Context, call mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		actor, _, err := tools.actor(ctx, call)
		if err != nil {
			return nil, err
		}
		scope, err := presentation.Scope(ctx, tools.Service.Store, actor.EnterpriseID, actor.SubjectID)
		if err != nil {
			return nil, err
		}
		value := presentation.PublicSnapshot(result.Structured)
		queryContext, _ := value["query_context"].(map[string]any)
		if queryContext == nil {
			queryContext = map[string]any{}
		}
		for _, key := range []string{"query", "document", "operation_name", "from", "to", "step_seconds", "lookback_seconds"} {
			if v, ok := call.Input[key]; ok {
				queryContext[key] = v
			}
		}
		value["query_context"] = queryContext
		data := presentation.BindData(metadata.ID, metadata.Discovery.Title, value)
		data["partial"] = result.Partial
		refs := []toolruntime.ResourceRef{}
		names := []map[string]any{}
		if call.Input["resource_ids"] != nil || call.Input["scope"] != nil {
			ids, err := queryResourceIDs(call.Input)
			if err != nil {
				return nil, err
			}
			if err := tools.Service.ValidateQueryScope(ctx, actor, ids); err != nil {
				return nil, err
			}
			names, refs, err = presentation.Resources(ctx, tools.Service.Store, actor.EnterpriseID, ids)
			if err != nil {
				return nil, err
			}
			if len(refs) != len(ids) {
				return nil, ErrQueryScope
			}
		}
		data["resources"] = names
		if metadata.ID == "telemetry.collector.list" || metadata.ID == "telemetry.collector.get" {
			ids := []uuid.UUID{}
			items, _ := data["items"].([]any)
			if collector, ok := data["collector"].(map[string]any); ok {
				items = append(items, collector)
			}
			for _, raw := range items {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := item["resource_id"].(string); ok {
					id, e := uuid.Parse(text)
					if e != nil {
						return nil, e
					}
					ids = append(ids, id)
				}
			}
			if e := tools.Service.ValidateQueryScope(ctx, actor, ids); e != nil {
				return nil, e
			}
			names, refs, err = presentation.Resources(ctx, tools.Service.Store, actor.EnterpriseID, ids)
			if err != nil {
				return nil, err
			}
			data["resources"] = names
		}
		if metadata.ID == "telemetry.promql.query" {
			presentation.MetricContext(data)
		}
		return presentation.Bound(asset, data, scope, refs), nil
	}
	return metadata
}
