package toolgateway

import (
	"context"
	"embed"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

//go:embed templates/*.html
var templates embed.FS

func (tools ResourceTools) presentationMetadata(metadata mcp.Metadata) mcp.Metadata {
	metadata.ToolFamily = metadata.ID
	metadata.OutputSchema = outputSchema(metadata.ID)
	name := "resource"
	if strings.HasSuffix(metadata.ID, ".preview") || strings.HasPrefix(metadata.ID, "pending_action.") {
		name = "preview"
	}
	if metadata.ID == "kubernetes.pod.logs" {
		name = "logs"
	}
	source, _ := templates.ReadFile("templates/" + name + ".html")
	asset := presentation.ToolAsset(metadata.ID, string(source))
	if asset == nil {
		return metadata
	}
	metadata.Template = asset
	metadata.Present = func(ctx context.Context, call mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		enterprise, err := uuid.Parse(call.Enterprise)
		if err != nil {
			return nil, err
		}
		user, err := uuid.Parse(call.Subject)
		if err != nil {
			return nil, err
		}
		access, err := presentation.CurrentAccess(ctx, tools.Store, enterprise, user)
		if err != nil {
			return nil, err
		}
		data := presentation.BindData(call.ToolID, metadata.Discovery.Title, result.Structured)
		data["request_context"] = presentation.PublicSnapshot(call.Input)
		data["partial"] = result.Partial
		refs := resourcePresentationRefs(metadata.ID, metadata.Risk, data, call.Input)
		refs, err = tools.bindPresentationResources(ctx, enterprise, metadata.ID, data, refs, access)
		if err != nil {
			return nil, err
		}
		return presentation.Bound(asset, data, access.Scope, refs), nil
	}
	return metadata
}

func resourcePresentationRefs(tool, risk string, data, input map[string]any) []toolruntime.ResourceRef {
	refs := []toolruntime.ResourceRef{}
	if risk != "read" {
		return refs
	}
	kind := ""
	switch {
	case strings.HasPrefix(tool, "host."):
		kind = "host"
	case strings.HasPrefix(tool, "kubernetes.cluster."):
		kind = "kubernetes_cluster"
	case strings.HasPrefix(tool, "connector."):
		kind = "connector"
	}
	add := func(kind string, value any) {
		id, ok := value.(string)
		if !ok || kind == "" {
			return
		}
		if _, err := uuid.Parse(id); err == nil {
			refs = append(refs, toolruntime.ResourceRef{Type: kind, ID: id})
		}
	}
	if kind != "" {
		add(kind, data["id"])
		if items, ok := data["items"].([]any); ok {
			for _, raw := range items {
				if row, ok := raw.(map[string]any); ok {
					add(kind, row["id"])
				}
			}
		}
	}
	if kind == "" && strings.HasPrefix(tool, "kubernetes.") {
		add("kubernetes_cluster", input["cluster_id"])
	}
	return refs
}
