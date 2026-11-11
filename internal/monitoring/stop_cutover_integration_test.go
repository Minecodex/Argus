package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/collectorstate"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresStopAdmitsVerifiedQueueUntilEffectiveCutover(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	created, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "Owned stop", TargetName: "Owned stop", Integration: "mysql", TargetIdentity: json.RawMessage(`{"endpoint":"owned-mysql.test:3306"}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}, json.RawMessage(`{"username":"owned","password":"fixture-only"}`))
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
	if err != nil {
		t.Fatal(err)
	}
	stage := func(revision int64, enabled bool) {
		t.Helper()
		bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: collector.String(), Collections: []configbundle.Collection{{ID: row.ID.String(), Integration: "mysql", TargetID: row.TargetID.String(), TargetType: row.TargetType, Revision: revision, AssignmentEpoch: 1, Enabled: enabled, HasCredential: true}}})
		f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,$3,'{}',$4,decode(repeat('00',32),'hex'),'applying')", uuid.New(), collector, revision, bundle)
		f.exec("UPDATE collector_instances SET desired_revision=$2,status='installing' WHERE id=$1", collector, revision)
	}
	ack := func(revision int64) {
		t.Helper()
		if err := f.s.Store.InTx(ctx, func(q *db.Queries) error {
			_, err := collectorstate.Apply(ctx, q, f.a.EnterpriseID, collector, revision, "configure", true, "")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	source := uuid.New()
	stage(1, true)
	err = f.s.Store.Queries.RegisterCollectionTelemetrySource(ctx, db.RegisterCollectionTelemetrySourceParams{ID: source, EnterpriseID: f.a.EnterpriseID, CollectorID: collector, Generation: instance.SourceGeneration, ResourceType: row.TargetType, ResourceID: row.TargetID, SourceKey: "host/mysql/" + row.ID.String(), SourceType: "mysql", Signals: []string{"metrics"}, ConfigRevision: 1, ConfigHash: bytes.Repeat([]byte{0}, 32), CapabilityVersion: "fixture", CollectionID: nullID(row.ID), CollectionRevision: pgtype.Int8{Int64: 1, Valid: true}, AssignmentEpoch: pgtype.Int8{Int64: 1, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	params := db.ResolveTelemetrySourceParams{EnterpriseID: f.a.EnterpriseID, CollectorID: collector, ID: source, ConfigRevision: 1}
	ack(1)
	if _, err = f.s.Store.Queries.ResolveTelemetrySource(ctx, params); err != nil {
		t.Fatal("verified source rejected", err)
	}
	row, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.apply(Mutation{Kind: "collection", Operation: "update", ID: row.ID, ExpectedVersion: row.Version, Collection: &CollectionInput{Name: row.Name, Integration: row.IntegrationKey, Target: ResourceRef{Kind: row.TargetType, ID: row.TargetID}, CollectorID: collector, Config: row.Config, Enabled: false, CredentialUpdate: "keep"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Store.Queries.ResolveTelemetrySource(ctx, params); err != nil {
		t.Fatal("stop intent discarded already accepted queue before runtime cutover", err)
	}
	// Actual revocation remains immediate while ordinary stop is pending.
	f.exec("UPDATE monitoring_collections SET status='archived' WHERE id=$1", row.ID)
	if _, err = f.s.Store.Queries.ResolveTelemetrySource(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("archived collection admitted queue", err)
	}
	f.exec("UPDATE monitoring_collections SET status='active' WHERE id=$1", row.ID)
	f.exec("UPDATE monitoring_collection_assignments SET valid_to=now() WHERE collection_id=$1", row.ID)
	if _, err = f.s.Store.Queries.ResolveTelemetrySource(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked assignment admitted queue", err)
	}
	f.exec("UPDATE monitoring_collection_assignments SET valid_to=NULL WHERE collection_id=$1", row.ID)
	stage(2, false)
	ack(2)
	if _, err = f.s.Store.Queries.ResolveTelemetrySource(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("effective stop allowed old source to keep writing", err)
	}
	var historical int
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM telemetry_sources WHERE id=$1", source).Scan(&historical); err != nil || historical != 1 {
		t.Fatal("stop deleted historical provenance", historical, err)
	}
}
