package telemetry

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestCatalogCursorIsBoundToQueryAndAuthorization(t *testing.T) {
	r := DataCatalogRequest{EnterpriseID: uuid.New(), ResourceIDs: []uuid.UUID{uuid.New()}, SourceKeys: []string{"source:1"}, AuthorizationVersion: 1, Signal: "logs", Kind: "values", Field: "service_name", From: time.Unix(1, 0), To: time.Unix(100, 0)}
	r.Cursor = writeCatalogCursor(r, "api")
	if after, err := readCatalogCursor(r); err != nil || after != "api" {
		t.Fatalf("cursor failed: %s %v", after, err)
	}
	for _, change := range []func(*DataCatalogRequest){
		func(r *DataCatalogRequest) { r.SubjectID = uuid.New() },
		func(r *DataCatalogRequest) { r.SubjectType = "service_account" },
		func(r *DataCatalogRequest) { r.SourceKeys = []string{"other:1"} },
		func(r *DataCatalogRequest) { r.ResourceIDs = []uuid.UUID{uuid.New()} },
		func(r *DataCatalogRequest) { r.AuthorizationVersion++ },
		func(r *DataCatalogRequest) { r.To = r.To.Add(time.Second) },
		func(r *DataCatalogRequest) { r.Search = "new" },
		func(r *DataCatalogRequest) { r.Field = "body" },
	} {
		copy := r
		change(&copy)
		if _, err := readCatalogCursor(copy); err == nil {
			t.Fatal("cursor reused after context changed")
		}
	}
	r.SelectedValues = []string{"current"}
	r.Limit = 20
	if _, err := readCatalogCursor(r); err != nil {
		t.Fatal("selection/page size should not change query scope")
	}
}
