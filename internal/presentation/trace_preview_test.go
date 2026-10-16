package presentation

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestAPMPreviewSurvivesBoundsWhileExactRowsRemainPageable(t *testing.T) {
	rows := []any{}
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 600; i++ {
		rows = append(rows, map[string]any{"sourceId": "source", "resourceId": "host", "serviceName": "orders", "timestamp": start.Add(time.Duration(i) * time.Second).Format(time.RFC3339), "samplesPerSecond": float64(i)})
	}
	doc := `query { queryAPMRED(bucketSeconds:1) { limited rows { sourceId resourceId serviceName timestamp samplesPerSecond } } }`
	data := BindData("telemetry.skywalking.trace", "Trace", map[string]any{"query_context": map[string]any{"document": doc}, "data": map[string]any{"queryAPMRED": map[string]any{"limited": true, "rows": rows}}})
	view := ViewData(data)["result"].([]any)[0].(map[string]any)
	points := view["chart_preview"].(map[string]any)["series"].([]any)[0].(map[string]any)["points"].([]any)
	if len(points) > 200 || points[len(points)-1].(map[string]any)["timestamp"] != "2026-10-14T10:09:59Z" || view["limited"] != true {
		t.Fatal("preview clipped the full window or lost business limit")
	}
	signer := pagination.Signer{Key: make([]byte, 32)}
	p := db.GetToolPresentationRow{ID: uuid.New(), TemplateHash: "hash", AuthorizationScope: "scope", CreatedAt: pgtype.Timestamptz{Time: start, Valid: true}}
	owner := pagination.Binding{Audience: "enterprise", EnterpriseID: "tenant", SubjectID: "owner", AuthorizationVersion: 1}
	count, cursor := 0, ""
	for {
		parameters := map[string]any{"collection": "result", "index": float64(0), "field": "rows", "limit": float64(100)}
		if cursor != "" {
			parameters["cursor"] = cursor
		}
		page, next, err := PageData(signer, owner, p, data, "v1", parameters)
		if err != nil {
			t.Fatal(err)
		}
		count += len(page["items"].([]any))
		cursor = next
		if cursor == "" {
			break
		}
	}
	if count != 600 || len(data["result"].([]any)[0].(map[string]any)["rows"].([]any)) != 600 {
		t.Fatal("sealed row paging was truncated", count)
	}
	// Exercise the same projection for an aliased root in a frozen dashboard.
	public := PublicSnapshot(map[string]any{"panels": []any{map[string]any{"targets": []any{map[string]any{"result_type": "apm_red", "data": map[string]any{"data": map[string]any{"aliased": map[string]any{"rows": rows}}}}}}}})
	DashboardObservationContext(public, nil)
	panel := ViewData(public)["panels"].([]any)[0].(map[string]any)
	target := panel["targets"].([]any)[0].(map[string]any)
	result := target["data"].(map[string]any)["data"].(map[string]any)["aliased"].(map[string]any)
	preview := result["chart_preview"].(map[string]any)
	last := preview["series"].([]any)[0].(map[string]any)["points"].([]any)
	if fmt.Sprint(last[len(last)-1].(map[string]any)["timestamp"]) != "2026-10-14T10:09:59Z" {
		t.Fatal("frozen dashboard preview clipped")
	}
	// The same declared read channel can page the frozen GraphQL root of a
	// dashboard target, without rerunning the query or trusting a URL.
	fullPanel := public["panels"].([]any)[0].(map[string]any)
	fullPanel["id"] = "red-panel"
	fullTarget := fullPanel["targets"].([]any)[0].(map[string]any)
	fullTarget["id"] = "red-target"
	page, _, err := PageData(signer, owner, p, public, "dashboard-v1", map[string]any{"panel_id": "red-panel", "target_id": "red-target", "collection": "result", "index": float64(0), "field": "rows", "limit": float64(100)})
	if err != nil || page["total"] != float64(600) || len(page["items"].([]any)) != 100 {
		t.Fatal("frozen APM dashboard paging unavailable", page, err)
	}
}
