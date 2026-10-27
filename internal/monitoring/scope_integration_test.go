package monitoring

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresScopeApplicationDependenciesAndExplicitEmpty(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	prod := f.application("Orders production", "production")
	testApp := f.application("Orders test", "testing")
	collector := f.collector()
	created, err := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "shared nginx", TargetName: "shared nginx", Integration: "nginx", TargetIdentity: json.RawMessage(`{"endpoint":"http://nginx/status"}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, Bindings: []BindingInput{{ApplicationID: prod.ID}, {ApplicationID: testApp.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	collection, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	resolver := observationscope.Resolver{Queries: f.s.Store.Queries}
	selection := observationscope.Selection{Mode: observationscope.AllAuthorized, Signal: "metrics", Applications: &observationscope.ApplicationSelection{Mode: observationscope.Explicit, IDs: []uuid.UUID{prod.ID}}}
	snapshot, err := resolver.Resolve(ctx, f.a, selection)
	if err != nil || len(snapshot.Resources) != 2 || snapshot.Partial || len(snapshot.Relations) != 1 {
		t.Fatalf("application dependencies: %+v %v", snapshot, err)
	}
	for _, item := range snapshot.Resources {
		if item.Kind == "monitoring_target" && item.Environment != "" {
			t.Fatal("shared resource inherited executing host environment")
		}
	}
	selection.Applications.IDs = append(selection.Applications.IDs, testApp.ID)
	shared, err := resolver.Resolve(ctx, f.a, selection)
	if err != nil || len(shared.Resources) != 3 {
		t.Fatalf("shared resource was duplicated: %+v %v", shared, err)
	}
	selection.Mode = observationscope.Explicit
	selection.IDs = []uuid.UUID{}
	empty, err := resolver.Resolve(ctx, f.a, selection)
	if err != nil || len(empty.Resources) != 0 {
		t.Fatalf("explicit empty widened: %+v %v", empty, err)
	}
	selection.Mode = observationscope.AllAuthorized
	selection.Applications.IDs = []uuid.UUID{}
	empty, err = resolver.Resolve(ctx, f.a, selection)
	if err != nil || len(empty.Resources) != 0 {
		t.Fatalf("empty application selection widened: %+v %v", empty, err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), f.a.EnterpriseID, f.other.SubjectID, prod.ID)
	selection.Applications.IDs = []uuid.UUID{prod.ID}
	partial, err := resolver.Resolve(ctx, f.other, selection)
	if err != nil || !partial.Partial || len(partial.Resources) != 1 || partial.Resources[0].ID != prod.ID {
		t.Fatalf("application granted resource access: %+v %v", partial, err)
	}
	selection.Mode = observationscope.Explicit
	selection.IDs = []uuid.UUID{collection.TargetID}
	if _, err = resolver.Resolve(ctx, f.other, selection); !errors.Is(err, observationscope.ErrDenied) {
		t.Fatalf("explicit ungranted target: %v", err)
	}
	selection.Mode = observationscope.AllAuthorized
	selection.IDs = nil
	selection.Signal = "logs"
	logs, err := resolver.Resolve(ctx, f.a, selection)
	if err != nil || len(logs.Resources) != 1 || logs.Resources[0].ID != prod.ID {
		t.Fatalf("business log scope borrowed dependencies: %+v %v", logs, err)
	}
	selection.Signal = "metrics"
	bindings, err := f.s.Store.Queries.ListMonitoringBindings(ctx, db.ListMonitoringBindingsParams{EnterpriseID: f.a.EnterpriseID, Column2: []uuid.UUID{prod.ID}})
	if err != nil || len(bindings) != 1 {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "binding", Operation: "delete", ID: bindings[0].ID, ExpectedVersion: bindings[0].Version}); err != nil {
		t.Fatal(err)
	}
	unlinked, err := resolver.Resolve(ctx, f.a, selection)
	if err != nil || len(unlinked.Resources) != 1 || unlinked.Signature == snapshot.Signature {
		t.Fatalf("binding removal did not change snapshot: %+v %v", unlinked, err)
	}
}
