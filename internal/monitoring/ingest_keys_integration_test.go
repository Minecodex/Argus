package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresApplicationIngestKeyLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	// Domain tests use a transaction-only runtime, never claim an installed SDK listener.
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['otlp','argus_application_auth','argus_application_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	app, err := f.apply(Mutation{Kind: "application", Operation: "create", Application: &ApplicationInput{Name: "SDK orders", Environment: "production"}})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := f.apply(Mutation{Kind: "endpoint", Operation: "create", Endpoint: &IngestEndpointInput{Name: "OTLP ingestion", ApplicationID: app.ResourceID, CollectorID: collector, Protocol: "otlp", Signals: []string{"traces", "logs"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	create := func() uuid.UUID {
		t.Helper()
		result, err := f.apply(Mutation{Kind: "ingest_key", Operation: "create", IngestKey: &IngestKeyInput{EndpointID: endpoint.ResourceID, ExpiresAt: time.Now().Add(24 * time.Hour)}})
		if err != nil {
			t.Fatal(err)
		}
		return result.ResourceID
	}
	key := create()
	if _, err = f.s.RevealIngestKey(ctx, f.other, key); !errors.Is(err, ErrDenied) {
		t.Fatalf("ungranted reveal: %v", err)
	}
	run := f.a
	run.RunID = uuid.New()
	if _, err = f.s.RevealIngestKey(ctx, run, key); !errors.Is(err, ErrDenied) {
		t.Fatalf("model reveal: %v", err)
	}
	token, err := f.s.RevealIngestKey(ctx, f.a, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RevealIngestKey(ctx, f.a, key); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeated reveal: %v", err)
	}
	row, err := AuthenticateIngestKey(ctx, f.s.Store.Queries, collector, f.a.EnterpriseID, "otlp", "traces", token)
	if err != nil || row.Environment != "production" || row.ApplicationID != app.ResourceID {
		t.Fatalf("trusted application: %+v %v", row, err)
	}
	for _, test := range []struct {
		collector, enterprise   uuid.UUID
		protocol, signal, token string
	}{
		{uuid.New(), f.a.EnterpriseID, "otlp", "traces", token}, {collector, uuid.New(), "otlp", "traces", token},
		{collector, f.a.EnterpriseID, "jaeger", "traces", token}, {collector, f.a.EnterpriseID, "otlp", "metrics", token},
		{collector, f.a.EnterpriseID, "otlp", "traces", strings.Replace(token, "argus_app.", "invalid.", 1)},
	} {
		if _, err = AuthenticateIngestKey(ctx, f.s.Store.Queries, test.collector, test.enterprise, test.protocol, test.signal, test.token); !errors.Is(err, ErrIngestRejected) {
			t.Fatalf("foreign key admitted: %v", err)
		}
	}
	var stored []byte
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(k)) FROM monitoring_ingest_keys k WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte(token)) {
		t.Fatal("plaintext key persisted")
	}
	var plans []byte
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT jsonb_agg(immutable_plan) FROM pending_action_plans WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&plans); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plans, []byte(token)) {
		t.Fatal("plaintext key in action plan")
	}
	second := create()
	replacement, err := f.s.RevealIngestKey(ctx, f.a, second)
	if err != nil {
		t.Fatal(err)
	}
	if token == replacement {
		t.Fatal("rotation reused value")
	}
	if _, err = f.apply(Mutation{Kind: "ingest_key", Operation: "revoke", ID: key, ExpectedVersion: 1, IngestKey: &IngestKeyInput{}}); err != nil {
		t.Fatal(err)
	}
	if _, err = AuthenticateIngestKey(ctx, f.s.Store.Queries, collector, f.a.EnterpriseID, "otlp", "traces", token); !errors.Is(err, ErrIngestRejected) {
		t.Fatalf("revoked key: %v", err)
	}
	if _, err = AuthenticateIngestKey(ctx, f.s.Store.Queries, collector, f.a.EnterpriseID, "otlp", "traces", replacement); err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "endpoint", Operation: "set_enabled", ID: endpoint.ResourceID, ExpectedVersion: 1, Endpoint: &IngestEndpointInput{Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	if _, err = AuthenticateIngestKey(ctx, f.s.Store.Queries, collector, f.a.EnterpriseID, "otlp", "traces", replacement); !errors.Is(err, ErrIngestRejected) {
		t.Fatalf("disabled endpoint: %v", err)
	}
	metadata, err := f.s.Store.Queries.ListMonitoringIngestKeys(ctx, db.ListMonitoringIngestKeysParams{EnterpriseID: f.a.EnterpriseID, EndpointID: endpoint.ResourceID, PageLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(metadata)
	if bytes.Contains(public, []byte("key_digest")) || bytes.Contains(public, []byte(token)) {
		t.Fatal("read metadata exposed secret")
	}
}
