package monitoring

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestPostgresDeclaredServiceHasNoFakeObservationAndSDKReusesIt(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	app := f.application("Log only application", "production")
	declared, err := f.apply(Mutation{Kind: "service", Operation: "create", Service: &ApplicationServiceInput{ApplicationID: app.ID, Namespace: "business", Name: "orders"}})
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringService(ctx, db.GetMonitoringServiceParams{EnterpriseID: f.a.EnterpriseID, ID: declared.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	if row.FirstSeenAt.Valid || row.LastSeenAt.Valid || serviceDTO(row).LastSeenAt != nil {
		t.Fatal("declaration manufactured a telemetry observation")
	}
	if _, err = f.s.Preview(ctx, f.other, Mutation{Kind: "service", Operation: "create", Service: &ApplicationServiceInput{ApplicationID: app.ID, Name: "forbidden"}}, uuid.NewString()); err == nil {
		t.Fatal("ungranted application declaration allowed")
	}
	collector := f.collector()
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['otlp','argus_application_auth','argus_application_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	endpoint, err := f.apply(Mutation{Kind: "endpoint", Operation: "create", Endpoint: &IngestEndpointInput{Name: "SDK", ApplicationID: app.ID, CollectorID: collector, Protocol: "otlp", Signals: []string{"logs"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := f.apply(Mutation{Kind: "ingest_key", Operation: "create", IngestKey: &IngestKeyInput{EndpointID: endpoint.ResourceID, ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	identities, err := f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "logs", []IngestObservation{{KeyID: key.ResourceID, EndpointID: endpoint.ResourceID, ApplicationID: app.ID, Protocol: "otlp", ServiceNamespace: "business", ServiceName: "orders", Runtime: "go"}})
	if err != nil || len(identities) != 1 || identities[0].ServiceID != declared.ResourceID {
		t.Fatalf("SDK split a declared identity: %v", err)
	}
	row, err = f.s.Store.Queries.GetMonitoringService(ctx, db.GetMonitoringServiceParams{EnterpriseID: f.a.EnterpriseID, ID: declared.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	if !row.FirstSeenAt.Valid || !row.LastSeenAt.Valid || row.Runtime != "go" || row.Version != 1 {
		t.Fatal("observed service metadata or configuration version is incorrect")
	}
	if _, err = f.s.Preview(ctx, f.a, Mutation{Kind: "service", Operation: "create", Service: &ApplicationServiceInput{ApplicationID: app.ID, Namespace: "business", Name: "orders"}}, uuid.NewString()); err == nil {
		t.Fatal("duplicate service declaration accepted")
	}
}
