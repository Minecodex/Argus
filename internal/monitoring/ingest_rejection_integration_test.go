package monitoring

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresRejectedSDKDiagnosticsRequireSecretPossessionAndCollectorBinding(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['otlp','argus_application_auth','argus_application_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	app := f.application("SDK diagnostic ownership", "production")
	endpoint, err := f.apply(Mutation{Kind: "endpoint", Operation: "create", Endpoint: &IngestEndpointInput{Name: "SDK diagnostics", ApplicationID: app.ID, CollectorID: collector, Protocol: "otlp", Signals: []string{"traces"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := f.apply(Mutation{Kind: "ingest_key", Operation: "create", IngestKey: &IngestKeyInput{EndpointID: endpoint.ResourceID, ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.s.RevealIngestKey(ctx, f.a, key.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "ingest_key", Operation: "revoke", ID: key.ResourceID, ExpectedVersion: 1, IngestKey: &IngestKeyInput{}}); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []struct {
		enterprise, collector uuid.UUID
		protocol, token       string
	}{
		{f.a.EnterpriseID, collector, "otlp", "argus_app." + key.ResourceID.String() + "." + strings.Repeat("x", 43)},
		{uuid.New(), collector, "otlp", token},
		{f.a.EnterpriseID, uuid.New(), "otlp", token},
		{f.a.EnterpriseID, collector, "jaeger", token},
	} {
		if err = f.s.ObserveRejectedIngestKey(ctx, attempt.enterprise, attempt.collector, attempt.protocol, attempt.token); err != nil {
			t.Fatal(err)
		}
		row, err := f.s.Store.Queries.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: f.a.EnterpriseID, ID: endpoint.ResourceID})
		if err != nil || row.LastRejectedAt.Valid || row.LastErrorCode != "" {
			t.Fatal("unverified rejection poisoned another endpoint", err)
		}
	}
	if err = f.s.ObserveRejectedIngestKey(ctx, f.a.EnterpriseID, collector, "otlp", token); err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: f.a.EnterpriseID, ID: endpoint.ResourceID})
	if err != nil || !row.LastRejectedAt.Valid || row.LastErrorCode != "APPLICATION_INGEST_KEY_REVOKED" {
		t.Fatal("verified revoked Key did not produce a bounded diagnostic", err)
	}
	if err = f.s.Store.Queries.ObserveApplicationIngest(ctx, db.ObserveApplicationIngestParams{EnterpriseID: f.a.EnterpriseID, ID: endpoint.ResourceID}); err != nil {
		t.Fatal(err)
	}
	row, err = f.s.Store.Queries.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: f.a.EnterpriseID, ID: endpoint.ResourceID})
	if err != nil || !row.LastReceivedAt.Valid || row.LastErrorCode != "" || !row.LastRejectedAt.Valid {
		t.Fatal("recovered ingestion lost its diagnostic history or retained an active error", err)
	}
}
