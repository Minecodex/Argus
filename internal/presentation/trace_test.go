package presentation

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestTraceViewsPagingIsBoundToEachRootAndGraphField(t *testing.T) {
	data := BindData("telemetry.skywalking.trace", "Trace", map[string]any{"query_context": map[string]any{"document": `query { a:queryAPMServices { rows { serviceName } } b:queryAPMTopology { nodes { id serviceName } edges { sourceNodeId targetNodeId } } }`}, "data": map[string]any{"a": map[string]any{"rows": []any{map[string]any{"serviceName": "one"}, map[string]any{"serviceName": "two"}}}, "b": map[string]any{"nodes": []any{map[string]any{"id": "n1", "serviceName": "one"}, map[string]any{"id": "n2", "serviceName": "two"}}, "edges": []any{}}}})
	if data["data"] != nil || len(data["result"].([]any)) != 2 {
		t.Fatal(data)
	}
	signer := pagination.Signer{Key: make([]byte, 32)}
	p := db.GetToolPresentationRow{ID: uuid.New(), TemplateHash: "hash", AuthorizationScope: "scope", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	owner := pagination.Binding{Audience: "enterprise", EnterpriseID: "tenant", SubjectID: "owner", AuthorizationVersion: 1}
	params := map[string]any{"collection": "result", "index": float64(0), "field": "rows", "limit": float64(1)}
	page, cursor, err := PageData(signer, owner, p, data, "v1", params)
	if err != nil || cursor == "" || page["total"] != float64(2) {
		t.Fatal(page, cursor, err)
	}
	for _, bad := range []map[string]any{{"collection": "result", "index": float64(1), "field": "nodes", "cursor": cursor}, {"collection": "result", "index": float64(999), "field": "rows"}, {"collection": "result", "index": float64(0), "field": "private"}} {
		if _, _, err = PageData(signer, owner, p, data, "v1", bad); err == nil {
			t.Fatal("cross-root cursor or out-of-bounds selector accepted", bad)
		}
	}
	params = map[string]any{"collection": "result", "index": float64(1), "field": "nodes", "limit": float64(1)}
	if _, _, err = PageData(signer, owner, p, data, "v1", params); err != nil {
		t.Fatal(err)
	}
}
