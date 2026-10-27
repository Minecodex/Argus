package toolgateway

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (t MonitoringTools) presentation(meta mcp.Metadata) mcp.Metadata {
	name := "resource"
	if strings.HasSuffix(meta.ID, ".preview") {
		name = "preview"
	}
	source, _ := templates.ReadFile("templates/" + name + ".html")
	asset := presentation.ToolAsset(meta.ID, string(source))
	meta.Template = asset
	meta.Present = func(ctx context.Context, c mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		// Background accounts receive model-safe structured results; only a
		// human-owned conversation can create an interactive presentation.
		if c.SubjectType == "service_account" {
			return nil, nil
		}
		a, err := t.actor(ctx, c)
		if err != nil {
			return nil, err
		}
		access, err := presentation.CurrentAccess(ctx, t.Base.Store, a.EnterpriseID, a.SubjectID)
		if err != nil {
			return nil, err
		}
		data := presentation.BindData(meta.ID, meta.Discovery.Title, result.Structured)
		data["request_context"] = presentation.PublicSnapshot(c.Input)
		refs := []toolruntime.ResourceRef{}
		seen := map[string]bool{}
		add := func(kind string, raw any) {
			id, ok := raw.(string)
			if !ok {
				return
			}
			ref := toolruntime.ResourceRef{Type: kind, ID: id}
			if access.AllowsResource(ref) && !seen[kind+":"+id] {
				refs = append(refs, ref)
				seen[kind+":"+id] = true
			}
		}
		if meta.Risk == "read" {
			add("application", c.Input["application_id"])
			add("kubernetes_cluster", c.Input["cluster_id"])
			add("kubernetes_namespace", c.Input["namespace_id"])
			if ids, ok := c.Input["application_ids"].([]any); ok {
				for _, id := range ids {
					add("application", id)
				}
			}
			if selection, ok := c.Input["namespaces"].(map[string]any); ok {
				if ids, ok := selection["ids"].([]any); ok {
					for _, id := range ids {
						add("kubernetes_namespace", id)
					}
				}
			}
			rows := []map[string]any{data}
			if items, ok := data["items"].([]any); ok {
				for _, raw := range items {
					if row, ok := raw.(map[string]any); ok {
						rows = append(rows, row)
					}
				}
			}
			for _, row := range rows {
				if meta.ID == "application.list" || meta.ID == "application.get" {
					add("application", row["id"])
				}
				if meta.ID == "monitoring.target.list" {
					add("monitoring_target", row["id"])
				}
				add("application", row["application_id"])
				if target, ok := row["target"].(map[string]any); ok {
					kind, _ := target["kind"].(string)
					add(kind, target["id"])
				}
			}
			if ids, ok := data["namespace_references"].(map[string]any); ok {
				for _, id := range ids {
					add("kubernetes_namespace", id)
				}
			}
			if meta.ID == "application.key.list" {
				id, e := uuid.Parse(stringValue(c.Input, "endpoint_id"))
				if e != nil {
					return nil, e
				}
				endpoint, e := t.Base.Store.Queries.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: id})
				if e != nil {
					return nil, e
				}
				add("application", endpoint.ApplicationID.String())
			}
		}
		ids := []uuid.UUID{}
		for _, ref := range refs {
			id, _ := uuid.Parse(ref.ID)
			ids = append(ids, id)
		}
		names, _, err := presentation.Resources(ctx, t.Base.Store, a.EnterpriseID, ids)
		if err != nil {
			return nil, err
		}
		data["resources"], data["_resource_refs"] = names, refs
		return presentation.Bound(asset, data, access.Scope, refs), nil
	}
	return meta
}
