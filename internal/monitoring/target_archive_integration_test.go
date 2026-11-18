package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresTargetArchiveRequiresDependenciesToEndAndPreservesIdentity(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	created, err := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{
		Name: "archive-source", TargetName: "archive-target", Integration: "nginx", CollectorID: collector,
		TargetIdentity: json.RawMessage(`{"endpoint":"http://archive.test/status"}`), Config: json.RawMessage(`{}`), Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	collection, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.s.Store.Queries.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: f.a.EnterpriseID, ID: collection.TargetID})
	if err != nil {
		t.Fatal(err)
	}
	m := Mutation{Kind: "target", Operation: "archive", ID: before.ID, ExpectedVersion: before.Version}
	if _, err = f.s.Preview(ctx, f.a, m, uuid.NewString()); !errors.Is(err, ErrDependency) {
		t.Fatalf("active collection must block archival: %v", err)
	}
	app := f.application("Archive dependency", "testing")
	binding, err := f.apply(Mutation{Kind: "binding", Operation: "create", Binding: &BindingInput{ApplicationID: app.ID, Target: ResourceRef{Kind: "monitoring_target", ID: before.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "archive", ID: collection.ID, ExpectedVersion: collection.Version, Collection: &CollectionInput{}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Preview(ctx, f.a, m, uuid.NewString()); !errors.Is(err, ErrDependency) {
		t.Fatalf("live application binding must block archival: %v", err)
	}
	if _, err = f.apply(Mutation{Kind: "binding", Operation: "delete", ID: binding.ResourceID, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	result, err := f.apply(m)
	if err != nil || result.ResourceType != "monitoring_target" || result.ResourceID != before.ID || result.ResourceVersion != before.Version+1 {
		t.Fatalf("normal preview/confirmation/commit failed: %+v %v", result, err)
	}
	after, err := f.s.Store.Queries.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: f.a.EnterpriseID, ID: before.ID})
	if err != nil || after.Status != "archived" || after.Name != before.Name || !bytes.Equal(before.IdentityConfig, after.IdentityConfig) {
		t.Fatalf("archival changed historical identity: %+v %v", after, err)
	}
	retained, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: collection.ID})
	if err != nil || retained.Status != "archived" || retained.TargetID != before.ID {
		t.Fatal("source history or target identity was removed", err)
	}
	if _, err = f.s.Preview(ctx, f.a, m, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatalf("old version was accepted after archival: %v", err)
	}
}

func TestPostgresTargetArchiveRevalidatesScopeVersionAndDependencies(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	id := uuid.New()
	f.exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'archive-owned','web','nginx','{\"endpoint\":\"http://archive.test/status\"}',$3)", id, f.a.EnterpriseID, f.a.SubjectID)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, id)
	m := Mutation{Kind: "target", Operation: "archive", ID: id, ExpectedVersion: 1}
	if _, err := f.s.Preview(ctx, f.other, m, uuid.NewString()); err == nil {
		t.Fatal("ungranted subject could archive a target")
	}
	stale := m
	stale.ExpectedVersion = 2
	if _, err := f.s.Preview(ctx, f.a, stale, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatal("incorrect version accepted", err)
	}
	app := f.application("Late dependency", "testing")
	plan, err := f.s.prepare(ctx, f.s.Store.Queries, f.a, m, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "binding", Operation: "create", Binding: &BindingInput{ApplicationID: app.ID, Target: ResourceRef{Kind: "monitoring_target", ID: id}}}); err != nil {
		t.Fatal(err)
	}
	err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
		_, commitErr := f.s.commitTargetArchive(ctx, q, plan)
		return commitErr
	})
	if !errors.Is(err, ErrDependency) {
		t.Fatal("dependency added after preview bypassed commit validation", err)
	}
}
