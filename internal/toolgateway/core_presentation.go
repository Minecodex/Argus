package toolgateway

import (
	"context"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func CoreTemplate(tool string) *toolruntime.TemplateAsset {
	source, _ := templates.ReadFile("templates/discovery.html")
	return presentation.ToolAsset(tool, string(source))
}

func (g *Gateway) BindPresentations(store *postgres.Store) {
	g.PresentationScope = func(ctx context.Context, p toolruntime.Principal) (string, error) {
		return presentation.Scope(ctx, store, p.EnterpriseID, p.UserID)
	}
}

func (g *Gateway) corePresentation(ctx context.Context, tool string, call toolruntime.Invocation, result toolruntime.Result) (toolruntime.Result, error) {
	if g.PresentationScope == nil || CoreTemplate(tool) == nil {
		return result, nil
	}
	scope, err := g.PresentationScope(ctx, call.Principal)
	if err != nil {
		return result, err
	}
	asset := CoreTemplate(tool)
	if asset == nil {
		return result, nil
	}
	data := presentation.BindData(tool, tool, result.Data)
	result.Presentation = presentation.Bound(asset, data, scope, nil)
	return result, nil
}
