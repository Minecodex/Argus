package telemetry

import (
	"context"
	"embed"
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
	asset := presentation.Asset(string(source), metadata.ID+"/v1")
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
		data := presentation.Details(result.Structured)
		data["partial"] = result.Partial
		return &toolruntime.Presentation{Template: asset.Source, Hash: asset.Hash, Version: asset.Version, Data: data, AuthorizationScope: scope, Status: "ready", Resources: []toolruntime.ResourceRef{}}, nil
	}
	return metadata
}
