package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type workloadResolverFunc func(context.Context, Actor, uuid.UUID, logcollection.Workload) (logcollection.Workload, error)

func (f workloadResolverFunc) ResolveLogWorkload(ctx context.Context, a Actor, cluster uuid.UUID, w logcollection.Workload) (logcollection.Workload, error) {
	return f(ctx, a, cluster, w)
}

func TestPostgresKubernetesLogPreviewReplayDoesNotRepeatExternalReads(t *testing.T) {
	f, m, _ := newWorkloadPreparationFixture(t)
	key := uuid.NewString()
	first, err := f.s.Preview(t.Context(), f.a, m, key)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Workloads = workloadResolverFunc(func(context.Context, Actor, uuid.UUID, logcollection.Workload) (logcollection.Workload, error) {
		t.Error("idempotent replay issued another external command")
		return logcollection.Workload{}, ErrUnavailable
	})
	second, err := f.s.Preview(t.Context(), f.a, m, key)
	if err != nil || second.ID != first.ID {
		t.Fatalf("preview replay: %v", err)
	}
	changed := m
	in := *m.Collection
	in.Name = "different request"
	changed.Collection = &in
	if _, err = f.s.Preview(t.Context(), f.a, changed, key); !errors.Is(err, postgres.ErrIdempotencyConflict) {
		t.Fatalf("changed replay: %v", err)
	}
}

func newWorkloadPreparationFixture(t *testing.T) (*fixture, Mutation, db.KubernetesNamespaceResource) {
	t.Helper()
	f := newFixture(t)
	collector, cluster := f.collector(), uuid.New()
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'preflight','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.a.EnterpriseID)
	f.exec("UPDATE collector_instances SET resource_type='kubernetes_cluster',resource_id=$2 WHERE id=$1", collector, cluster)
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['filelog','file_storage','k8sattributes','argus_log_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'telemetry.collector.manage' FROM role_bindings WHERE enterprise_id=$1", f.a.EnterpriseID)
	ns, err := f.s.Store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: "orders", NamespaceUid: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, ns.ID)
	f.s.Workloads = workloadResolverFunc(func(_ context.Context, _ Actor, _ uuid.UUID, w logcollection.Workload) (logcollection.Workload, error) {
		w.Namespace, w.NamespaceUID = ns.Name, ns.NamespaceUid
		return w, nil
	})
	raw, _ := json.Marshal(logcollection.Config{Scope: "component", Workload: &logcollection.Workload{NamespaceID: ns.ID, Type: "deployment", Name: "orders", UID: uuid.NewString(), Containers: []string{"main"}}})
	m := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "Orders logs", Integration: "k8slog", CollectorID: collector, Enabled: true, Config: raw}}
	return f, m, ns
}

func confirmPreparedWorkload(t *testing.T, f *fixture, m Mutation) (resource.Service, actionservice.Confirmation) {
	t.Helper()
	p, err := f.s.Preview(t.Context(), f.a, m, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the production outer extension, which must forward preparation.
	r := resource.Service{Store: f.s.Store, Actions: f.s.Actions, Extension: kubernetesworkbench.ActionExtension{Next: remoteexecution.Service{Next: ActionExtension{Service: f.s}}}}
	workflow := actionservice.Service{Store: f.s.Store, Idempotency: f.s.Actions.Idempotency, Resources: r}
	confirmed, err := workflow.Confirm(t.Context(), f.a.SubjectID.String(), uuid.NewString(), f.a.EnterpriseID, f.a.AuthorizationVersion, false, p.ActionRef, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return r, confirmed
}

func TestPostgresKubernetesLogPreparationOutsideActionTransaction(t *testing.T) {
	f, m, _ := newWorkloadPreparationFixture(t)
	original := f.s.Workloads
	calls := 0
	var execution uuid.UUID
	f.s.Workloads = workloadResolverFunc(func(ctx context.Context, a Actor, cluster uuid.UUID, w logcollection.Workload) (logcollection.Workload, error) {
		calls++
		{
			// A Connector query needs an independent write transaction. Taking
			// this lock would fail immediately if the outer Action held it while
			// waiting for the remote read, as the previous implementation did.
			tx, err := f.s.Store.Pool.Begin(ctx)
			if err != nil {
				return w, err
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, "SELECT id FROM pending_actions WHERE enterprise_id=$1 FOR UPDATE NOWAIT", a.EnterpriseID); err != nil {
				return w, err
			}
			if execution != uuid.Nil {
				if _, err = tx.Exec(ctx, "SELECT id FROM executions WHERE id=$1 FOR UPDATE NOWAIT", execution); err != nil {
					return w, err
				}
			}
			if err = tx.Commit(ctx); err != nil {
				return w, err
			}
		}
		return original.ResolveLogWorkload(ctx, a, cluster, w)
	})
	r, confirmed := confirmPreparedWorkload(t, f, m)
	execution = confirmed.Execution.ID
	payload, _ := json.Marshal(actionservice.ExecutionTask{EnterpriseID: f.a.EnterpriseID, ExecutionID: execution})
	if err := (actionservice.Executor{Store: f.s.Store, Resources: r}).Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	state, err := f.s.Store.Queries.GetExecution(t.Context(), db.GetExecutionParams{EnterpriseID: f.a.EnterpriseID, ID: execution})
	if err != nil || state.Status != "succeeded" || calls != 3 {
		t.Fatalf("execution=%s external_reads=%d error=%v", state.Status, calls, err)
	}
	// Re-delivery of a finished task never performs another external read.
	if err = (actionservice.Executor{Store: f.s.Store, Resources: r}).Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil || calls != 3 {
		t.Fatalf("task replay: reads=%d error=%v", calls, err)
	}
}

func TestPostgresKubernetesLogPreparedSnapshotCannotBypassRevalidation(t *testing.T) {
	for _, scenario := range []string{"missing", "expired", "namespace_recreated", "namespace_version", "revoked", "different_action", "changed_plan"} {
		t.Run(scenario, func(t *testing.T) {
			f, m, ns := newWorkloadPreparationFixture(t)
			r, confirmed := confirmPreparedWorkload(t, f, m)
			prepared, err := r.PreparePendingAction(t.Context(), confirmed.PendingAction)
			if err != nil {
				t.Fatal(err)
			}
			expected := resource.ErrActionInvalidated
			switch scenario {
			case "missing":
				prepared = t.Context()
			case "expired":
				now := time.Now().UTC().Add(31 * time.Second)
				f.s.Now = func() time.Time { return now }
				r.Extension = ActionExtension{Service: f.s}
			case "namespace_recreated":
				f.exec("UPDATE kubernetes_namespace_resources SET namespace_uid=$2 WHERE id=$1", ns.ID, uuid.NewString())
			case "namespace_version":
				f.exec("UPDATE kubernetes_namespace_resources SET version=version+1 WHERE id=$1", ns.ID)
			case "revoked":
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", f.a.EnterpriseID, ns.ID)
			case "different_action":
				snapshot := prepared.Value(workloadSnapshotKey{}).(workloadSnapshot)
				snapshot.actionID = uuid.New()
				prepared = context.WithValue(prepared, workloadSnapshotKey{}, snapshot)
			case "changed_plan":
				f.exec("UPDATE pending_action_plans SET immutable_plan=jsonb_set(immutable_plan,'{mutation,collection,name}','\"tampered\"') WHERE pending_action_id=$1", confirmed.PendingAction.ID)
			}
			err = f.s.Store.InTx(t.Context(), func(q *db.Queries) error {
				_, err := r.ExecutePendingAction(prepared, q, confirmed.PendingAction)
				return err
			})
			if !errors.Is(err, expected) {
				t.Fatalf("expected %v; got %v", expected, err)
			}
			var count int
			if err = f.s.Store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM monitoring_collections WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected preflight committed a collection: %d %v", count, err)
			}
			token, err := f.s.Store.Queries.GetPendingActionTokenForUpdate(t.Context(), db.GetPendingActionTokenForUpdateParams{EnterpriseID: f.a.EnterpriseID, ActionRef: confirmed.PendingAction.ActionRef})
			if err != nil || token.Status != "active" {
				t.Fatalf("rejected preflight consumed private token: %s %v", token.Status, err)
			}
		})
	}
}
