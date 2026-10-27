package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/collectorstate"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresMonitoringLeaseAndOutcomeFencing(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	created, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "mysql", TargetName: "mysql", Integration: "mysql", TargetIdentity: json.RawMessage(`{"endpoint":"mysql:3306"}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}, json.RawMessage(`{"username":"monitor","password":"test-${env:HOME}-only"}`))
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	bundle := configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: collector.String(), Collections: []configbundle.Collection{{ID: row.ID.String(), Integration: "mysql", TargetID: row.TargetID.String(), TargetType: "monitoring_target", Revision: 1, AssignmentEpoch: 1, Enabled: true, HasCredential: true}}}
	raw, _ := json.Marshal(bundle)
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}',$3,$4,'applying')", uuid.New(), collector, raw, bytes.Repeat([]byte{1}, 32))
	delivery := secret.MonitoringDelivery{CollectionID: row.ID, CollectionRevision: 1, AssignmentEpoch: 1, CollectorID: collector, CollectorRevision: 1, OperationRef: "controlled-runtime-test", RecipientType: "connector", RecipientID: "collector-connector", TTL: time.Minute}
	issue := func() db.CredentialLease {
		t.Helper()
		var lease db.CredentialLease
		err := f.s.Store.InTx(ctx, func(q *db.Queries) error {
			var e error
			lease, e = f.s.Credentials.PrepareMonitoringLeaseWithQueries(ctx, q, f.a.SubjectID.String(), f.a.EnterpriseID, delivery)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
		return lease
	}
	lease := issue()
	if _, err = f.s.Credentials.FulfillLease(ctx, f.a.EnterpriseID, lease.ID, "connector", "other-connector"); !errors.Is(err, secret.ErrInvalidLease) {
		t.Fatalf("recipient mismatch accepted: %v", err)
	}
	material, err := f.s.Credentials.FulfillLease(ctx, f.a.EnterpriseID, lease.ID, "connector", delivery.RecipientID)
	if err != nil || !bytes.Contains(material.Value, []byte("test-${env:HOME}-only")) {
		t.Fatalf("credential delivery: %v", err)
	}
	clear(material.Value)
	if _, err = f.s.Credentials.FulfillLease(ctx, f.a.EnterpriseID, lease.ID, "connector", delivery.RecipientID); !errors.Is(err, secret.ErrInvalidLease) {
		t.Fatalf("lease replay accepted: %v", err)
	}
	private, err := f.s.Credentials.MonitoringConfigForCollector(ctx, f.a.SubjectID.String(), f.a.EnterpriseID, "connector", delivery.RecipientID, "test-config", 1, raw)
	if err != nil || !bytes.Contains(private, []byte("test-${env:HOME}-only")) {
		t.Fatalf("private overlay: %v", err)
	}
	clear(private)
	if err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
		applied, e := collectorstate.Apply(ctx, q, f.a.EnterpriseID, collector, 1, "configure", true, "")
		if e == nil && !applied {
			t.Error("current revision ignored")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	observed, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil || observed.EffectiveRevision != 1 || observed.Health != "unknown" || observed.LastSampleAt.Valid {
		t.Fatalf("deployment fabricated samples or health: %+v %v", observed, err)
	}
	samples, _ := json.Marshal([]map[string]any{{"collection_id": row.ID, "revision": 1, "epoch": 1}})
	if touched, e := f.s.Store.Queries.ObserveMonitoringSamples(ctx, db.ObserveMonitoringSamplesParams{EnterpriseID: f.a.EnterpriseID, CollectorID: nullID(collector), Samples: samples}); e != nil || touched != 1 {
		t.Fatalf("current sample observation: %d %v", touched, e)
	}
	observed, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil || !observed.LastSampleAt.Valid || observed.Health != "healthy" {
		t.Fatalf("sample not observed: %v", err)
	}
	lastGood := observed.LastSampleAt.Time
	failureAt := observed.LastAttemptAt.Time.Add(time.Second)
	observeDiagnostic := func(health, code string, hasData bool, at time.Time) int64 {
		t.Helper()
		value := map[string]any{"collection_id": row.ID, "revision": 1, "epoch": 1, "health": health, "error_code": code, "has_data": hasData, "observed_at": at}
		if hasData {
			value["data_at"] = at
		}
		raw, _ := json.Marshal([]map[string]any{value})
		count, e := f.s.Store.Queries.ObserveMonitoringSamples(ctx, db.ObserveMonitoringSamplesParams{EnterpriseID: f.a.EnterpriseID, CollectorID: nullID(collector), Samples: raw})
		if e != nil {
			t.Fatal(e)
		}
		return count
	}
	if observeDiagnostic("failed", "COLLECTION_SCRAPE_FAILED", false, failureAt) != 1 {
		t.Fatal("failure observation not applied")
	}
	observed, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil || observed.Health != "failed" || observed.ErrorCode != "COLLECTION_SCRAPE_FAILED" || !observed.LastSampleAt.Time.Equal(lastGood) {
		t.Fatal("scrape failure fabricated a fresh target sample or healthy state")
	}
	if observeDiagnostic("healthy", "", true, failureAt.Add(-time.Millisecond)) != 0 {
		t.Fatal("queued old success overwrote a newer failure")
	}
	recoveryAt := failureAt.Add(time.Second)
	if observeDiagnostic("healthy", "", true, recoveryAt) != 1 {
		t.Fatal("recovery observation not applied")
	}
	observed, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil || observed.Health != "healthy" || observed.ErrorCode != "" || !observed.LastSampleAt.Time.Equal(recoveryAt) {
		t.Fatal("actual recovery did not clear scrape failure and preserve sample time")
	}
	stale := issue()
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: row.ID, ExpectedVersion: 1, Collection: &CollectionInput{Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	if touched, e := f.s.Store.Queries.ObserveMonitoringSamples(ctx, db.ObserveMonitoringSamplesParams{EnterpriseID: f.a.EnterpriseID, CollectorID: nullID(collector), Samples: samples}); e != nil || touched != 0 {
		t.Fatalf("disabled sample updated health: %d %v", touched, e)
	}
	if _, err = f.s.Credentials.FulfillLease(ctx, f.a.EnterpriseID, stale.ID, "connector", delivery.RecipientID); !errors.Is(err, secret.ErrInvalidLease) {
		t.Fatalf("disabled collection redeemed stale lease: %v", err)
	}
	f.exec("UPDATE collector_instances SET desired_revision=2,status='installing' WHERE id=$1", collector)
	if err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
		applied, e := collectorstate.Apply(ctx, q, f.a.EnterpriseID, collector, 1, "configure", false, "OLD_RESULT")
		if applied {
			t.Error("late failure changed newer revision")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
	if err != nil || current.Status != "installing" || current.DesiredRevision != 2 || current.EffectiveRevision != 1 {
		t.Fatalf("collector revision fencing: %+v %v", current, err)
	}
}
