package presentation

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestSealedPagingBindsPrincipalPresentationVersionAndSlice(t *testing.T) {
	signer := pagination.Signer{Key: make([]byte, 32)}
	p := db.GetToolPresentationRow{ID: uuid.New(), TemplateHash: "hash", AuthorizationScope: "scope", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	owner := pagination.Binding{SubjectID: "owner", EnterpriseID: "tenant", Audience: "enterprise", AuthorizationVersion: 3}
	data := map[string]any{"items": []any{map[string]any{"id": "one", "values": []any{[]any{1, "4"}, []any{2, "5"}, []any{3, "6"}}}, map[string]any{"id": "two"}}}
	_, next, err := PageData(signer, owner, p, data, "version1", map[string]any{"limit": float64(1)})
	if err != nil || next == "" {
		t.Fatal(next, err)
	}
	for _, change := range []string{"subject", "presentation", "version", "scope", "slice"} {
		o, q, version := owner, p, "version1"
		params := map[string]any{"cursor": next}
		switch change {
		case "subject":
			o.SubjectID = "other"
		case "presentation":
			q.ID = uuid.New()
		case "version":
			version = "version2"
		case "scope":
			q.AuthorizationScope = "revoked"
		case "slice":
			params["field"], params["index"] = "values", float64(0)
		}
		if _, _, e := PageData(signer, o, q, data, version, params); e == nil {
			t.Fatal("accepted cursor across", change)
		}
	}
	second, _, err := PageData(signer, owner, p, data, "version1", map[string]any{"cursor": next})
	if err != nil || second["offset"] != float64(1) {
		t.Fatal(second, err)
	}
	shard, next, err := PageData(signer, owner, p, data, "version1", map[string]any{"index": float64(0), "field": "values", "limit": float64(2)})
	if err != nil || next == "" || shard["total"] != float64(3) {
		t.Fatal(shard, next, err)
	}
	for _, params := range []map[string]any{{"url": "other"}, {"resource_id": "other"}, {"limit": float64(0)}, {"field": "password", "index": float64(0)}, {"cursor": "forged"}, {"panel_id": "outside", "target_id": "q"}} {
		if _, _, e := PageData(signer, owner, p, data, "version1", params); e == nil {
			t.Fatal("accepted unbound parameters", params)
		}
	}
}

func TestPreviewRetainsWholeWindowLatestExtremaAndExactStatistics(t *testing.T) {
	points := []any{}
	for i := 0; i < 1000; i++ {
		points = append(points, []any{float64(i), float64(i % 50)})
	}
	points[501] = []any{float64(501), float64(999)}
	full := map[string]any{"items": []any{map[string]any{"values": points}}}
	view := ViewData(full)
	row := view["items"].([]any)[0].(map[string]any)
	samples := row["values"].([]any)
	if len(samples) > 200 || samples[0].([]any)[0] != float64(0) || samples[len(samples)-1].([]any)[0] != float64(999) {
		t.Fatal("window truncated", len(samples))
	}
	stats := row["argus_statistics"].(map[string]any)
	if stats["max"] != float64(999) || stats["last"] != float64(49) || stats["sample_count"] != float64(1000) {
		t.Fatal(stats)
	}
	raw, _ := json.Marshal(full)
	if len(full["items"].([]any)[0].(map[string]any)["values"].([]any)) != 1000 || len(raw) == 0 {
		t.Fatal("sealed result was mutated")
	}
}

func TestPublicReadSnapshotStripsAuthorityWithNormalizedKeys(t *testing.T) {
	data := PublicSnapshot(map[string]any{"Password": "private", "nested": map[string]any{"COMMIT_TOKEN": "authority", "name": "<script>data</script>"}})
	if data["Password"] != nil || data["nested"].(map[string]any)["COMMIT_TOKEN"] != nil {
		t.Fatal(data)
	}
	if data["nested"].(map[string]any)["name"] == nil {
		t.Fatal("public data lost")
	}
}

func TestV2PreviewPreservesProtocolNestedDashboardSamplesAndLabels(t *testing.T) {
	series := map[string]any{"metric": map[string]any{"__name__": "cpu", "argus_resource_id": "bound-host"}, "values": []any{[]any{float64(1), "2"}, []any{float64(2), "4"}}}
	full := map[string]any{"panels": []any{map[string]any{"id": "cpu", "targets": []any{map[string]any{"id": "q", "data": map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{series}}}}}}}}
	view := ViewData(full)
	panel := view["panels"].([]any)[0].(map[string]any)
	target := panel["targets"].([]any)[0].(map[string]any)
	data := target["data"].(map[string]any)["data"].(map[string]any)
	row := data["result"].([]any)[0].(map[string]any)
	if row["metric"].(map[string]any)["argus_resource_id"] != "bound-host" || row["values"].([]any)[1].([]any)[1] != "4" {
		t.Fatal("nested frozen series was silently replaced with omitted text", row)
	}
	if view["_presentation_truncated"] != nil {
		t.Fatal("small dashboard must be fully renderable", view)
	}
}

func TestLargeDashboardPreviewKeepsAllPanelsAndExactSealedReductions(t *testing.T) {
	panels := []any{}
	for panel := 0; panel < 11; panel++ {
		rows := []any{}
		for series := 0; series < 30; series++ {
			points := []any{}
			for i := 0; i < 1000; i++ {
				points = append(points, []any{float64(i), float64(i)})
			}
			rows = append(rows, map[string]any{"metric": map[string]any{"__name__": "observed_metric", "resource_id": "named-host"}, "values": points})
		}
		panels = append(panels, map[string]any{"id": panel, "type": "timeseries", "targets": []any{map[string]any{"id": "q", "data": map[string]any{"data": map[string]any{"result": rows}}}}})
	}
	full := map[string]any{"name": "Frozen dashboard", "panels": panels}
	view := ViewData(full)
	encoded, _ := json.Marshal(view)
	if len(encoded) > 512<<10 || len(view["panels"].([]any)) != 11 {
		t.Fatal("large result lost frozen panel structure", len(encoded))
	}
	for _, raw := range view["panels"].([]any) {
		panel := raw.(map[string]any)
		data := panel["targets"].([]any)[0].(map[string]any)["data"].(map[string]any)["data"].(map[string]any)
		row := data["result"].([]any)[0].(map[string]any)
		if row["argus_statistics"].(map[string]any)["sample_count"] != float64(1000) || row["argus_statistics"].(map[string]any)["last"] != float64(999) {
			t.Fatal("preview changed exact statistics", row)
		}
		points := row["values"].([]any)
		if points[0].([]any)[0] != float64(0) || points[len(points)-1].([]any)[0] != float64(999) {
			t.Fatal("preview clipped the time range")
		}
	}
}
