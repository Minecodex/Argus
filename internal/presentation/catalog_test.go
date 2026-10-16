package presentation

import "testing"

func TestCatalogUsesActualCollectionAndFreezesValueRows(t *testing.T) {
	for _, kind := range []string{"metrics", "fields", "values", "resources"} {
		t.Run(kind, func(t *testing.T) {
			data := map[string]any{"metrics": []any{}, "fields": []any{}, "values": []any{}}
			row := any(map[string]any{"name": "orders"})
			if kind == "values" {
				row = "orders"
			}
			data[kind] = []any{row}
			NormalizeItems("telemetry-catalog", data)
			items := data["items"].([]any)
			if data["catalog_kind"] != kind || len(items) != 1 {
				t.Fatal(data)
			}
			if kind == "values" && items[0].(map[string]any)["value"] != "orders" {
				t.Fatal(items)
			}
			NormalizeCatalog(data, "fields")
			if data["catalog_kind"] != "fields" || len(data["items"].([]any)) != len(data["fields"].([]any)) {
				t.Fatal("explicit empty kind lost", data)
			}
		})
	}
}
