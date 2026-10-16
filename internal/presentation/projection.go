package presentation

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func Resources(ctx context.Context, store *postgres.Store, enterprise uuid.UUID, ids []uuid.UUID) ([]map[string]any, []toolruntime.ResourceRef, error) {
	rows, err := store.Queries.ResolvePresentationResources(ctx, db.ResolvePresentationResourcesParams{EnterpriseID: enterprise, Ids: ids})
	if err != nil {
		return nil, nil, err
	}
	names := []map[string]any{}
	refs := []toolruntime.ResourceRef{}
	for _, row := range rows {
		names = append(names, map[string]any{"id": row.ID.String(), "name": row.Name, "type": row.ResourceType})
		refs = append(refs, toolruntime.ResourceRef{ID: row.ID.String(), Type: row.ResourceType})
	}
	return names, refs, nil
}

// Normalize only result collection shape, not display decisions. Paging reads
// this frozen collection; it cannot continue a business query with new filters.
func NormalizeItems(entry string, data map[string]any) {
	if _, ok := data["items"].([]any); ok {
		return
	}
	payload, _ := data["data"].(map[string]any)
	page, _ := data["page"].(map[string]any)
	var items any
	switch entry {
	case "metric-query":
		items = payload["result"]
		if payload["resultType"] == "scalar" {
			items = []any{map[string]any{"metric": map[string]any{}, "value": items}}
		}
	case "log-query":
		items = data["data"]
		if payload != nil {
			items = payload["rows"]
		}
	case "pod-logs":
		if content, ok := data["content"].(string); ok {
			rows := []any{}
			for i, line := range strings.Split(content, "\n") {
				rows = append(rows, map[string]any{"line": i + 1, "body": line})
			}
			items = rows
		}
	case "trace-result":
		normalizeTrace(data, payload)
		if data["result"] != nil {
			return
		}
		for _, key := range []string{"queryBasicTraces", "queryTraces"} {
			if result, ok := payload[key].(map[string]any); ok {
				items = result["traces"]
			}
		}
		if items == nil {
			items = payload["traces"]
		}
		if item, ok := payload["queryTrace"].(map[string]any); ok {
			items = []any{item}
		}
	case "telemetry-catalog":
		NormalizeCatalog(data, "")
		return
	default:
		items = page["items"]
	}
	if rows, ok := items.([]any); ok {
		data["items"] = rows
	}
}

// Associate series only through source-provided labels. A missing resource label
// denotes the authorized aggregate/query scope, never an invented host.
func MetricContext(data map[string]any) {
	items, _ := data["items"].([]any)
	resources, _ := data["resources"].([]map[string]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		metric, _ := item["metric"].(map[string]any)
		info := map[string]any{"scope": "aggregate", "metric_name": metric["__name__"], "source": metric["source_key"]}
		if unit, ok := metric["unit"].(string); ok {
			info["unit"] = unit
		}
		id, _ := metric["resource_id"].(string)
		if id == "" {
			id, _ = metric["argus_resource_id"].(string)
		}
		if id != "" {
			info["scope"] = "unidentified"
			for _, resource := range resources {
				if resource["id"] == id {
					info["scope"], info["resource_name"] = "resource", resource["name"]
				}
			}
		}
		objects := []string{}
		for _, key := range []string{"db_name", "database", "db.system", "service_name", "service", "device", "mountpoint", "instance"} {
			if value, ok := metric[key].(string); ok && value != "" {
				objects = append(objects, key+"="+value)
			}
		}
		info["business_object"] = strings.Join(objects, " · ")
		item["presentation_context"] = info
		parts := []string{}
		if name, ok := info["resource_name"].(string); ok && name != "" {
			parts = append(parts, name)
		}
		parts = append(parts, objects...)
		if name, ok := info["metric_name"].(string); ok && name != "" {
			parts = append(parts, name)
		}
		if len(parts) > 0 {
			item["display_name"] = strings.Join(parts, " · ")
		}
	}
}

func DashboardObservationContext(data map[string]any, resources []map[string]any) {
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			if item["result_type"] == "apm_red" {
				skywalking.AttachREDPreviews(item["data"])
			}
			if metric, ok := item["metric"].(map[string]any); ok && metric != nil {
				MetricContext(map[string]any{"items": []any{item}, "resources": resources})
			}
			for _, child := range item {
				visit(child)
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(data)
}
