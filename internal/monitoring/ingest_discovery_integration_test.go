package monitoring

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresSDKConcurrentDiscoveryAndApplicationBoundaries(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	ctx := t.Context()
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['otlp','argus_application_auth','argus_application_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	app, err := f.apply(Mutation{Kind: "application", Operation: "create", Application: &ApplicationInput{Name: "Concurrent orders", Environment: "production"}})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := f.apply(Mutation{Kind: "endpoint", Operation: "create", Endpoint: &IngestEndpointInput{Name: "OTLP", ApplicationID: app.ResourceID, CollectorID: collector, Protocol: "otlp", Signals: []string{"traces"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := f.apply(Mutation{Kind: "ingest_key", Operation: "create", IngestKey: &IngestKeyInput{EndpointID: endpoint.ResourceID, ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	observation := IngestObservation{KeyID: key.ResourceID, EndpointID: endpoint.ResourceID, ApplicationID: app.ResourceID, Protocol: "otlp", ServiceName: "orders", ServiceNamespace: "business", Runtime: "go", InstanceKey: "replica-a"}
	var wg sync.WaitGroup
	results := make(chan []ApplicationIdentity, 12)
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, e := f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "traces", []IngestObservation{observation})
			if e != nil {
				failures <- e
			} else {
				results <- result
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var service, instance uuid.UUID
	for result := range results {
		if len(result) != 1 || result[0].Environment != "production" {
			t.Fatal("missing trusted context")
		}
		if service != uuid.Nil && (service != result[0].ServiceID || instance != result[0].InstanceID) {
			t.Fatal("concurrent discovery split identity")
		}
		service, instance = result[0].ServiceID, result[0].InstanceID
	}
	if service == uuid.Nil || instance == uuid.Nil {
		t.Fatal("no discovery result")
	}
	bad := observation
	bad.Environment = "testing"
	if _, err = f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "traces", []IngestObservation{bad}); !errors.Is(err, ErrIngestRejected) {
		t.Fatalf("SDK environment override: %v", err)
	}
	bad = observation
	bad.ApplicationID = uuid.New()
	if _, err = f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "traces", []IngestObservation{bad}); !errors.Is(err, ErrIngestRejected) {
		t.Fatalf("SDK application override: %v", err)
	}
	valid := observation
	valid.ServiceName = "must-rollback"
	bad = observation
	bad.ServiceName = "unknown_service:go"
	if _, err = f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "traces", []IngestObservation{valid, bad}); !errors.Is(err, ErrIngestRejected) {
		t.Fatalf("invalid mixed batch: %v", err)
	}
	count, err := f.s.Store.Queries.CountMonitoringServices(ctx, db.CountMonitoringServicesParams{EnterpriseID: f.a.EnterpriseID, ApplicationID: app.ResourceID})
	if err != nil || count != 1 {
		t.Fatalf("partial registration: %d %v", count, err)
	}
	observation.InstanceKey = ""
	result, err := f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "traces", []IngestObservation{observation})
	if err != nil || result[0].ServiceID != service || result[0].InstanceID != uuid.Nil {
		t.Fatal("missing instance rejected or fabricated")
	}
}
