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

func TestResultFilteringSearchesAllSealedRowsAndBindsCursor(t *testing.T) {
	items := []any{}
	for i := 0; i < 120; i++ {
		items = append(items, map[string]any{"name": fmt.Sprintf("host-%03d", i), "status": "connected", "environment": "prod"})
	}
	items[110].(map[string]any)["status"] = "offline"
	signer := pagination.Signer{Key: make([]byte, 32)}
	owner := pagination.Binding{SubjectID: "user", EnterpriseID: "tenant", Audience: "enterprise", AuthorizationVersion: 2}
	p := db.GetToolPresentationRow{ID: uuid.New(), TemplateHash: "hash", AuthorizationScope: "scope", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	data := map[string]any{"items": items}
	page, _, err := PageData(signer, owner, p, data, "v1", map[string]any{"search": "host-110"})
	if err != nil || page["total"] != float64(1) || page["source_total"] != float64(120) {
		t.Fatal(page, err)
	}
	page, _, err = PageData(signer, owner, p, data, "v1", map[string]any{"filters": map[string]any{"status": "offline"}})
	if err != nil || page["items"].([]any)[0].(map[string]any)["name"] != "host-110" {
		t.Fatal(page, err)
	}
	_, cursor, err := PageData(signer, owner, p, data, "v1", map[string]any{"sort": "name", "direction": "desc", "limit": float64(1)})
	if err != nil || cursor == "" {
		t.Fatal(cursor, err)
	}
	for _, params := range []map[string]any{{"sort": "name", "direction": "asc"}, {"search": "host-1"}, {"filters": map[string]any{"environment": "prod"}}} {
		params["cursor"] = cursor
		if _, _, err := PageData(signer, owner, p, data, "v1", params); err == nil {
			t.Fatal("accepted changed filter cursor", params)
		}
	}
	page, _, err = PageData(signer, owner, p, data, "v1", map[string]any{"sort": "name", "direction": "desc", "limit": float64(1), "cursor": cursor})
	if err != nil || page["items"].([]any)[0].(map[string]any)["name"] != "host-118" {
		t.Fatal(page, err)
	}
	if items[0].(map[string]any)["name"] != "host-000" {
		t.Fatal("mutated sealed rows")
	}
}

func TestResultFiltersRejectExpressionsAuthorityFieldsAndNestedSliceFiltering(t *testing.T) {
	items, _, _, err := filterPageItems([]any{map[string]any{"name": "public", "Password": "hidden-credential"}}, map[string]any{"search": "hidden-credential"})
	if err != nil || len(items) != 0 {
		t.Fatal("search leaked a private field through result counts", items, err)
	}
	for _, params := range []map[string]any{{"search": true}, {"filters": map[string]any{"password": "x"}}, {"sort": "name;DROP"}, {"direction": "backwards"}, {"filters": []any{}}} {
		if _, _, _, err := filterPageItems(nil, params); err == nil {
			t.Fatal(params)
		}
	}
}
