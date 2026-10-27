package monitoring

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresLogRulesOwnershipAndExclusiveClaims(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'telemetry.collector.manage' FROM role_bindings WHERE enterprise_id=$1", f.a.EnterpriseID)
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['filelog','journald','file_storage'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	app := f.application("Business logs", "production")
	service, err := f.apply(Mutation{Kind: "service", Operation: "create", Service: &ApplicationServiceInput{ApplicationID: app.ID, Name: "orders"}})
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, config string) Mutation {
		return Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: name, Integration: "filelog", CollectorID: collector, ApplicationID: app.ID, ServiceID: service.ResourceID, Enabled: true, Config: json.RawMessage(config)}}
	}
	first, err := f.apply(create("Orders", `{"scope":"business","include":["/var/log/orders/**/*.log"],"exclude":["/var/log/orders/private/**"]}`))
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: first.ResourceID})
	if err != nil || row.TargetType != "application" || row.TargetID != app.ID || row.ServiceID.UUID != service.ResourceID || row.CredentialID.Valid {
		t.Fatalf("business ownership: %+v %v", row, err)
	}
	var bindings int
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_application_bindings WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatalf("application self-binding: %d %v", bindings, err)
	}
	if _, err = f.apply(create("Intersecting", `{"scope":"business","include":["/var/log/orders/2026/api?.log"]}`)); !errors.Is(err, ErrLogOverlap) {
		t.Fatalf("intersecting paths accepted: %v", err)
	}
	// Exclusions are a real set subtraction, not a string equality test.
	if _, err = f.apply(create("Excluded directory", `{"scope":"business","include":["/var/log/orders/private/*.log"]}`)); err != nil {
		t.Fatal(err)
	}
	disabled := create("Disabled overlap", `{"scope":"business","include":["/var/log/orders/api.log"]}`)
	disabled.Collection.Enabled = false
	second, err := f.apply(disabled)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: second.ResourceID, ExpectedVersion: 1, Collection: &CollectionInput{Enabled: true}}); !errors.Is(err, ErrLogOverlap) {
		t.Fatalf("enable missed overlap: %v", err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: first.ResourceID, ExpectedVersion: 1, Collection: &CollectionInput{Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: second.ResourceID, ExpectedVersion: 1, Collection: &CollectionInput{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	// A second Collector on the same physical host cannot claim the same file.
	another := uuid.New()
	if _, err = f.s.Store.Pool.Exec(ctx, "INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision) SELECT $1,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision FROM collector_instances WHERE id=$2", another, collector); err == nil {
		t.Fatal("physical Collector identity duplicated")
	}
	missingService := create("Missing service", `{"scope":"business","include":["/var/log/new.log"]}`)
	missingService.Collection.ServiceID = uuid.Nil
	if _, err = f.apply(missingService); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unmapped business logs accepted: %v", err)
	}
	system := create("System", `{"scope":"system","include":["/var/log/system.log"]}`)
	if _, err = f.apply(system); !errors.Is(err, ErrInvalid) {
		t.Fatalf("system assigned an application: %v", err)
	}
	system.Collection.ApplicationID, system.Collection.ServiceID = uuid.Nil, uuid.Nil
	created, err := f.apply(system)
	if err != nil {
		t.Fatal(err)
	}
	row, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil || row.TargetType != "host" || row.ApplicationID.Valid {
		t.Fatalf("system ownership: %+v %v", row, err)
	}
	if _, err = f.s.Preview(ctx, f.other, system, uuid.NewString()); err == nil {
		t.Fatal("ungranted host accepted")
	}
	journal := create("Business journal", `{"scope":"business","units":["orders","orders.service"]}`)
	journal.Collection.Integration = "journald"
	journalResult, err := f.apply(journal)
	if err != nil {
		t.Fatal(err)
	}
	row, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: journalResult.ResourceID})
	if err != nil || row.TargetType != "application" || row.TargetID != app.ID || row.ServiceID.UUID != service.ResourceID {
		t.Fatalf("journal mapping: %v", err)
	}
	journal.Collection.Config = json.RawMessage(`{"scope":"business","units":["orders.service"]}`)
	if _, err = f.apply(journal); !errors.Is(err, ErrLogOverlap) {
		t.Fatalf("normalized journal units overlap: %v", err)
	}
	journal.Collection.Config = json.RawMessage(`{"scope":"business"}`)
	if _, err = f.apply(journal); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded business journal accepted: %v", err)
	}
}
