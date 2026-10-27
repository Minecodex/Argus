package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/collectorstate"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresCollectionMigrationKeepsVerifiedExecutionUntilCutover(t *testing.T) {
	for _, scenario := range []string{"failed_candidate", "successful_cutover", "retirement_revoked", "retirement_placement_returned", "retirement_inflight_config"} {
		succeeds := scenario != "failed_candidate"
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			ctx := t.Context()
			a := f.collector()
			f.exec("UPDATE hosts SET name='Original execution host' WHERE id=(SELECT resource_id FROM collector_instances WHERE id=$1)", a)
			b := f.collector()
			created, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "Migration database", TargetName: "Migration database", Integration: "mysql", TargetIdentity: json.RawMessage(`{"endpoint":"mysql.test:3306"}`), CollectorID: a, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}, json.RawMessage(`{"username":"monitor","password":"old-verified-test-only"}`))
			if err != nil {
				t.Fatal(err)
			}
			row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
			if err != nil {
				t.Fatal(err)
			}
			oldCredential := row.CredentialID.UUID
			configs := map[uuid.UUID][]byte{}
			sources := map[uuid.UUID]uuid.UUID{}
			stage := func(collector uuid.UUID, logical, epoch int64) {
				t.Helper()
				bundle := configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: collector.String(), Collections: []configbundle.Collection{{ID: row.ID.String(), Integration: "mysql", TargetID: row.TargetID.String(), TargetType: "monitoring_target", Revision: logical, AssignmentEpoch: epoch, Enabled: true, HasCredential: true}}}
				raw, _ := json.Marshal(bundle)
				configs[collector] = raw
				f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}',$3,decode(repeat('00',32),'hex'),'applying')", uuid.New(), collector, raw)
				instance, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
				if err != nil {
					t.Fatal(err)
				}
				source := uuid.New()
				sources[collector] = source
				err = f.s.Store.Queries.RegisterCollectionTelemetrySource(ctx, db.RegisterCollectionTelemetrySourceParams{ID: source, EnterpriseID: f.a.EnterpriseID, CollectorID: collector, Generation: instance.SourceGeneration, ResourceType: row.TargetType, ResourceID: row.TargetID, SourceKey: "host/mysql/" + row.ID.String(), SourceType: "mysql", Signals: []string{"metrics"}, ConfigRevision: 1, ConfigHash: bytes.Repeat([]byte{0}, 32), CapabilityVersion: "v1", CollectionID: nullID(row.ID), CollectionRevision: pgtype.Int8{Int64: logical, Valid: true}, AssignmentEpoch: pgtype.Int8{Int64: epoch, Valid: true}})
				if err != nil {
					t.Fatal(err)
				}
			}
			ack := func(collector uuid.UUID, success bool) {
				t.Helper()
				if err := f.s.Store.InTx(ctx, func(q *db.Queries) error {
					_, err := collectorstate.Apply(ctx, q, f.a.EnterpriseID, collector, 1, "configure", success, "CANDIDATE_FAILED")
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			allowed := func(collector uuid.UUID) bool {
				_, err := f.s.Store.Queries.ResolveTelemetrySource(ctx, db.ResolveTelemetrySourceParams{EnterpriseID: f.a.EnterpriseID, CollectorID: collector, ID: sources[collector], ConfigRevision: 1})
				return err == nil
			}
			stage(a, 1, 1)
			ack(a, true)
			if !allowed(a) {
				t.Fatal("verified original assignment rejected")
			}
			updated, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "update", ID: row.ID, ExpectedVersion: 1, Collection: &CollectionInput{Name: row.Name, Integration: row.IntegrationKey, Target: ResourceRef{Kind: row.TargetType, ID: row.TargetID}, CollectorID: b, Config: row.Config, Enabled: true, CredentialUpdate: "replace"}}, json.RawMessage(`{"username":"monitor","password":"new-candidate-test-only"}`))
			if err != nil || updated.ResourceVersion != 2 {
				t.Fatal("migration intent failed", err)
			}
			current, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
			if err != nil || current.CollectorID.UUID != b || current.EffectiveCollectorID.UUID != a || current.EffectiveAssignmentEpoch != 1 || !allowed(a) {
				t.Fatal("candidate prematurely replaced the original execution", err)
			}
			credential, err := f.s.Store.Queries.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: f.a.EnterpriseID, ID: oldCredential})
			if err != nil || credential.Status != "active" {
				t.Fatal("verified credential retired before cutover", err)
			}
			private, err := f.s.Credentials.MonitoringConfigForCollector(ctx, f.a.SubjectID.String(), f.a.EnterpriseID, "connector", "old-recipient", "old-verified-runtime", 1, configs[a])
			if err != nil || !bytes.Contains(private, []byte("old-verified-test-only")) || bytes.Contains(private, []byte("new-candidate-test-only")) {
				t.Fatal("last verified runtime lost its authorized credential", err)
			}
			clear(private)
			stage(b, 2, 2)
			if allowed(b) {
				t.Fatal("candidate admitted before its effective configuration")
			}
			ack(b, succeeds)
			current, err = f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: row.ID})
			if err != nil {
				t.Fatal(err)
			}
			if succeeds {
				if !allowed(b) || allowed(a) || current.EffectiveCollectorID.UUID != b || current.EffectiveAssignmentEpoch != 2 {
					t.Fatal("cutover did not fence the previous execution")
				}
				credential, err = f.s.Store.Queries.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: f.a.EnterpriseID, ID: oldCredential})
				if err != nil || credential.Status != "disabled" {
					t.Fatal("unused old credential was not retired", err)
				}
				var count int
				if err := f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_assignment_retirements WHERE collection_id=$1 AND previous_collector_id=$2 AND status='pending'", row.ID, a).Scan(&count); err != nil || count != 1 {
					t.Fatal("old executor retirement was not durably queued", err)
				}
				var retirement uuid.UUID
				if err := f.s.Store.Pool.QueryRow(ctx, "SELECT id FROM monitoring_assignment_retirements WHERE collection_id=$1 AND previous_collector_id=$2", row.ID, a).Scan(&retirement); err != nil {
					t.Fatal(err)
				}
				payload, _ := json.Marshal(map[string]uuid.UUID{"enterprise_id": f.a.EnterpriseID, "id": retirement})

				task := runtime.Task{RuntimeTask: db.RuntimeTask{EnterpriseID: nullID(f.a.EnterpriseID), Payload: payload}}
				worker := AssignmentRetirementWorker{Service: f.s}
				if scenario == "retirement_revoked" {
					f.exec("DELETE FROM role_bindings WHERE enterprise_id=$1", f.a.EnterpriseID)
					if err := worker.Handle(ctx, task); err != nil {
						t.Fatal(err)
					}
					state, err := f.s.Store.Queries.GetMonitoringAssignmentRetirement(ctx, db.GetMonitoringAssignmentRetirementParams{EnterpriseID: f.a.EnterpriseID, ID: retirement})
					if err != nil || state.Status != "blocked" || state.ErrorCode != "COLLECTION_RETIRE_AUTHORIZATION_CHANGED" {
						t.Fatal("revoked retirement dispatched", err)
					}
					return
				}
				if scenario == "retirement_placement_returned" {
					f.exec("UPDATE monitoring_collections SET collector_id=$2 WHERE id=$1", row.ID, a)
					f.s.Runtime = nil
					if err := (AssignmentRetirementWorker{Service: f.s}).Handle(ctx, task); err != nil {
						t.Fatal(err)
					}
					state, err := f.s.Store.Queries.GetMonitoringAssignmentRetirement(ctx, db.GetMonitoringAssignmentRetirementParams{EnterpriseID: f.a.EnterpriseID, ID: retirement})
					if err != nil || state.Status != "completed" {
						t.Fatal("superseded retirement did not settle", err)
					}
					return
				}
				if scenario == "retirement_inflight_config" {
					f.exec("UPDATE collector_instances SET desired_revision=2 WHERE id=$1", a)
					if err := worker.Handle(ctx, task); !errors.Is(err, ErrUnavailable) {
						t.Fatal("in-flight config was overwritten", err)
					}
					state, err := f.s.Store.Queries.GetMonitoringAssignmentRetirement(ctx, db.GetMonitoringAssignmentRetirementParams{EnterpriseID: f.a.EnterpriseID, ID: retirement})
					if err != nil || state.Status != "pending" {
						t.Fatal("in-flight config lost its pending retirement", err)
					}
					return
				}
				// Hold the same collection lock used by an outcome. Once the
				// worker is waiting on it, the retirement lock must remain free.
				blocker, err := f.s.Store.Pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer blocker.Rollback(context.Background())
				var pid int
				if err := blocker.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
					t.Fatal(err)
				}
				if _, err := blocker.Exec(ctx, "SELECT id FROM monitoring_collections WHERE id=$1 FOR UPDATE", row.ID); err != nil {
					t.Fatal(err)
				}
				waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- worker.Handle(waitCtx, task) }()
				for {
					var waiting bool
					if err := f.s.Store.Pool.QueryRow(waitCtx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", pid).Scan(&waiting); err != nil {
						t.Fatal(err)
					}
					if waiting {
						break
					}
					select {
					case err := <-done:
						t.Fatal("worker did not wait for the collection lock", err)
					case <-waitCtx.Done():
						t.Fatal(waitCtx.Err())
					case <-time.After(10 * time.Millisecond):
					}
				}
				if _, err := blocker.Exec(waitCtx, "SELECT id FROM monitoring_assignment_retirements WHERE id=$1 FOR UPDATE NOWAIT", retirement); err != nil {
					t.Fatal("retirement locked before collection", err)
				}
				if err := blocker.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
				if err := <-done; err != nil {
					t.Fatal("retirement dispatch failed", err)
				}

				retired, err := f.s.Store.Queries.LockMonitoringAssignmentRetirement(ctx, db.LockMonitoringAssignmentRetirementParams{EnterpriseID: f.a.EnterpriseID, ID: retirement})
				if err != nil || retired.Status != "dispatched" {
					t.Fatal("retirement completed without observing the old executor", err)
				}
				empty, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: a.String(), Collections: []configbundle.Collection{}})
				f.exec("UPDATE collector_instances SET desired_revision=2 WHERE id=$1", a)
				f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,2,'{}',$3,decode(repeat('00',32),'hex'),'applying')", uuid.New(), a, empty)
				if err := f.s.Store.InTx(ctx, func(q *db.Queries) error {
					_, err := collectorstate.Apply(ctx, q, f.a.EnterpriseID, a, 2, "configure", true, "")
					return err
				}); err != nil {
					t.Fatal(err)
				}
				retired, err = f.s.Store.Queries.LockMonitoringAssignmentRetirement(ctx, db.LockMonitoringAssignmentRetirementParams{EnterpriseID: f.a.EnterpriseID, ID: retirement})
				if err != nil || retired.Status != "completed" {
					t.Fatal("verified old configuration removal did not settle retirement", err)
				}
				for _, check := range []struct {
					collector uuid.UUID
					want      bool
				}{{a, false}, {b, true}} {
					used, err := f.s.Store.Queries.HasCollectorMonitoringDependencies(ctx, db.HasCollectorMonitoringDependenciesParams{EnterpriseID: f.a.EnterpriseID, CollectorID: nullID(check.collector)})
					if err != nil || !used.Valid || used.Bool != check.want {
						t.Fatal("uninstall dependencies ignored verified placement or retirement", err)
					}
				}
			} else if !allowed(a) || allowed(b) || current.EffectiveCollectorID.UUID != a || current.ApplyStatus != "failed" {
				t.Fatal("failed candidate replaced the last verified execution")
			}
		})
	}
}
