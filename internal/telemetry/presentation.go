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
		value["query_context"] = map[string]any{"query": call.Input["query"], "document": call.Input["document"], "operation_name": call.Input["operation_name"], "from": call.Input["from"], "to": call.Input["to"], "step_seconds": call.Input["step_seconds"], "lookback_seconds": call.Input["lookback_seconds"]}
		data := presentation.BindData(metadata.ID, metadata.Discovery.Title, value)
		data["partial"] = result.Partial
		refs := []toolruntime.ResourceRef{}
		names := []map[string]any{}
		if call.Input["resource_ids"] != nil {
			ids, err := inputUUIDs(call.Input["resource_ids"])
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
