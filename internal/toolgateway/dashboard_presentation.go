package toolgateway

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (tools DashboardTools) queryPresentation(metadata mcp.Metadata) mcp.Metadata {
	metadata.Template = dashboard.QueryTemplate()
	metadata.Present = func(ctx context.Context, call mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		// Intermediate polling does not append duplicate progress presentations.
		if call.ToolID == "telemetry.dashboard.query.get" && result.Structured["manifest"] == nil && result.Structured["status"] != "failed" && result.Structured["status"] != "cancelled" {
			return nil, nil
		}
		c, err := tools.scope(ctx, call)
		if err != nil {
			return nil, err
		}
		id, err := uuid.Parse(stringValue(result.Structured, "id"))
		if err != nil || tools.Jobs == nil {
			return nil, mcp.ErrInputInvalid
		}
		return tools.Jobs.PresentQuery(ctx, c.actor, c.principal.ConversationID, id)
	}
	return metadata
}

func (tools DashboardTools) definitionPresentation(metadata mcp.Metadata) mcp.Metadata {
	binding, err := presentation.ToolBinding(metadata.ID)
	if err != nil || binding.Mode != "template" {
		return metadata
	}
	name := "dashboard-definition"
	if binding.Entry == "change-preview" {
		name = "preview"
	}
	source, _ := templates.ReadFile("templates/" + name + ".html")
	asset := presentation.ToolAsset(metadata.ID, string(source))
	metadata.Template = asset
	metadata.Present = func(ctx context.Context, call mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		c, err := tools.scope(ctx, call)
		if err != nil {
			return nil, err
		}
		scope, err := presentation.Scope(ctx, tools.Service.Store, c.actor.EnterpriseID, c.actor.SubjectID)
		if err != nil {
			return nil, err
		}
		data := presentation.BindData(metadata.ID, metadata.Discovery.Title, result.Structured)
		data["request_context"] = presentation.PublicSnapshot(call.Input)
		data["partial"] = result.Partial
		if binding.Entry == "telemetry-catalog" {
			kind, _ := call.Input["kind"].(string)
			if metadata.ID == "telemetry.dashboard.catalog.resources" {
				kind = "resources"
			}
			presentation.NormalizeCatalog(data, kind)
			data["catalog_context"] = presentation.PublicSnapshot(call.Input)
		}
		return presentation.Bound(asset, data, scope, nil), nil
	}
	return metadata
}
