package toolgateway

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func connectorRelations(row map[string]any) []toolruntime.ResourceRef {
	refs := []toolruntime.ResourceRef{}
	for _, field := range []struct{ key, kind string }{{"host_id", "host"}, {"kubernetes_cluster_id", "kubernetes_cluster"}} {
		id, ok := row[field.key].(string)
		if parsed, err := uuid.Parse(id); ok && err == nil && parsed != uuid.Nil {
			refs = append(refs, toolruntime.ResourceRef{Type: field.kind, ID: id})
		}
	}
	return refs
}

// Enrich all authorized relationships in one lookup. Unavailable combines
// missing grants, missing permission and deleted objects to avoid an existence
// oracle. Only resolved, authorized relations become navigation capabilities.
func (tools ResourceTools) bindPresentationResources(ctx context.Context, enterprise uuid.UUID, tool string, data map[string]any, refs []toolruntime.ResourceRef, access presentation.Access) ([]toolruntime.ResourceRef, error) {
	rows := []map[string]any{}
	if strings.HasPrefix(tool, "connector.") {
		if items, ok := data["items"].([]any); ok {
			for _, raw := range items {
				if row, ok := raw.(map[string]any); ok {
					rows = append(rows, row)
				}
			}
		} else {
			rows = append(rows, data)
		}
	}
	candidates := append([]toolruntime.ResourceRef{}, refs...)
	for _, row := range rows {
		candidates = append(candidates, connectorRelations(row)...)
	}
	allowed := map[string]bool{}
	ids := []uuid.UUID{}
	seenID := map[uuid.UUID]bool{}
	for _, ref := range candidates {
		if !access.AllowsResource(ref) {
			continue
		}
		id, _ := uuid.Parse(ref.ID)
		allowed[ref.Type+":"+ref.ID] = true
		if !seenID[id] {
			ids = append(ids, id)
			seenID[id] = true
		}
	}
	names, _, err := presentation.Resources(ctx, tools.Store, enterprise, ids)
	if err != nil {
		return nil, err
	}
	resolved := map[string]map[string]any{}
	resources := []map[string]any{}
	for _, item := range names {
		key := item["type"].(string) + ":" + item["id"].(string)
		if allowed[key] {
			resolved[key] = item
			resources = append(resources, item)
		}
	}
	seenRef := map[string]bool{}
	for _, ref := range refs {
		seenRef[ref.Type+":"+ref.ID] = true
	}
	for _, row := range rows {
		relations := []any{}
		for _, ref := range connectorRelations(row) {
			item := map[string]any{"id": ref.ID, "type": ref.Type, "status": "unavailable"}
			if name := resolved[ref.Type+":"+ref.ID]; name != nil {
				item["name"], item["status"] = name["name"], "available"
				if !seenRef[ref.Type+":"+ref.ID] {
					refs = append(refs, ref)
					seenRef[ref.Type+":"+ref.ID] = true
				}
			}
			relations = append(relations, item)
		}
		row["related_resources"] = relations
		if id, ok := row["id"].(string); ok {
			if name, ok := row["name"].(string); ok && name != "" {
				resources = append(resources, map[string]any{"id": id, "type": "connector", "name": name})
			}
		}
	}
	data["resources"], data["_resource_refs"] = resources, refs
	return refs, nil
}
