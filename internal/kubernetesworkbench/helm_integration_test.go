package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type helmFixtureBackend struct {
	inventoryFixtureBackend
	service          *Service
	objects          []kuberneteshelm.Object
	writes, previews int
	lost             bool
	revoke           func()
}

func (b *helmFixtureBackend) HelmReleases(context.Context, db.KubernetesCluster, kuberneteshelm.Request) ([]kuberneteshelm.Release, error) {
	return []kuberneteshelm.Release{}, nil
}
func (b *helmFixtureBackend) PreviewHelm(ctx context.Context, c db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	b.previews++
	m, e := b.service.Credentials.LoadHelmInput(ctx, c.EnterpriseID, c.ID, r, true, "server", c.ID.String())
	if e != nil {
		return kuberneteshelm.Result{}, e
	}
	defer func() { clear(m.Chart); clear(m.Values); clear(m.Key) }()
	return kuberneteshelm.Result{SchemaVersion: kuberneteshelm.SchemaVersion, DryRun: true, Objects: b.objects, SealedPlan: []byte("controlled-native-seal")}, nil
}
func (b *helmFixtureBackend) ExecuteHelm(ctx context.Context, c db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	id := uuid.MustParse(r.OperationID)
	w := kuberneteshelm.Write{Version: "v1", Resource: "configmaps", Namespace: "orders", Name: "settings", Verb: "PATCH"}
	if e := b.service.ValidateHelmWrite(ctx, c.EnterpriseID, c.ID, id, r, w); e != nil {
		return kuberneteshelm.Result{}, e
	}
	b.writes++
	if b.revoke != nil {
		b.revoke()
		if e := b.service.ValidateHelmWrite(ctx, c.EnterpriseID, c.ID, id, r, w); e == nil {
			return kuberneteshelm.Result{}, errors.New("revoked write allowed")
		}
		return kuberneteshelm.Result{SchemaVersion: kuberneteshelm.SchemaVersion, Accepted: true, Applied: true, ErrorCode: kuberneteshelm.ErrDenied.Error()}, nil
	}
	if b.lost {
		return kuberneteshelm.Result{}, kuberneteshelm.ErrUnknown
	}
	return b.ObserveHelm(ctx, c, r)
}
func (b *helmFixtureBackend) ObserveHelm(_ context.Context, _ db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	return kuberneteshelm.Result{SchemaVersion: kuberneteshelm.SchemaVersion, Accepted: true, Applied: true, Ready: !b.lost, Release: kuberneteshelm.Release{Name: r.Name, Namespace: r.Namespace.Name, Revision: r.ExpectedRevision + 1, Status: "deployed"}}, nil
}
func newHelmFixture(t *testing.T) (*mutationFixture, *helmFixtureBackend, HelmInput) {
	f := newMutationFixture(t)
	connector := uuid.New()
	f.exec("INSERT INTO connectors(id,enterprise_id,role,name,kubernetes_cluster_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at,capabilities) VALUES($1,$2,'kubernetes','helm-test',$3,$4,decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'online',1,now()+interval '1 hour',$5)", connector, f.actor.EnterpriseID, f.cluster, connector.String(), []string{kuberneteshelm.SchemaVersion})
	f.exec("UPDATE kubernetes_clusters SET connector_id=$2 WHERE id=$1", f.cluster, connector)
	f.exec("INSERT INTO connector_sessions(connector_id,enterprise_id,gateway_instance_id,connection_epoch,capabilities,connected_at,last_heartbeat_at) VALUES($1,$2,'gateway',1,$3,now(),now())", connector, f.actor.EnterpriseID, []string{kuberneteshelm.SchemaVersion})
	grantStream(f, "kubernetes.helm.manage")
	d, _ := kubernetesinventory.Lookup("configmap")
	b := &helmFixtureBackend{service: &f.service, objects: []kuberneteshelm.Object{{Definition: d, Namespace: "orders", NamespaceUID: "orders-uid", Name: "settings", Action: "apply", Validation: "server", Changes: []string{"/data/*"}}}}
	f.service.Backend = b
	return f, b, HelmInput{NamespaceID: f.namespace, Operation: "install", Name: "sample", TimeoutSeconds: 120, Reason: "Validate protected lifecycle"}
}
func confirmHelm(f *mutationFixture, pending db.PendingAction) uuid.UUID {
	f.t.Helper()
	ctx := f.t.Context()
	r := resource.Service{Store: f.service.Store, Actions: f.service.Actions, Extension: ActionExtension{Service: f.service}}
	workflow := actionservice.Service{Store: f.service.Store, Idempotency: f.service.Actions.Idempotency, Resources: r}
	confirmed, e := workflow.Confirm(ctx, f.actor.SubjectID.String(), uuid.NewString(), f.actor.EnterpriseID, f.actor.AuthorizationVersion, false, pending.ActionRef, uuid.NewString())
	if e != nil || confirmed.Execution == nil {
		f.t.Fatal("confirmation", e)
	}
	raw, _ := json.Marshal(actionservice.ExecutionTask{EnterpriseID: f.actor.EnterpriseID, ExecutionID: confirmed.Execution.ID})
	if e = (actionservice.Executor{Store: f.service.Store, Resources: r}).Handle(ctx, runtime.Task{RuntimeTask: db.RuntimeTask{Payload: raw}}); e != nil {
		f.t.Fatal(e)
	}
	execution, e := f.service.Store.Queries.GetExecution(ctx, db.GetExecutionParams{ID: confirmed.Execution.ID, EnterpriseID: f.actor.EnterpriseID})
	if e != nil || !execution.KubernetesOperationID.Valid {
		f.t.Fatal("operation not queued", e, execution.ErrorCode)
	}
	return execution.KubernetesOperationID.UUID
}
func TestPostgresHelmPrivatePreviewReplayAndInputBinding(t *testing.T) {
	f, b, in := newHelmFixture(t)
	ctx := t.Context()
	key := uuid.NewString()
	chart, values := []byte("protected-chart-canary"), []byte("password: protected-values-canary")
	first, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, chart, values, key)
	if e != nil {
		t.Fatal(e)
	}
	second, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, chart, values, key)
	if e != nil || second.ID != first.ID || b.previews != 1 {
		t.Fatal("preview replay rerendered", e, b.previews)
	}
	if _, e = f.service.PreviewHelm(ctx, f.actor, f.cluster, in, chart, []byte("different"), key); !errors.Is(e, postgres.ErrIdempotencyConflict) {
		t.Fatal("changed private input reused", e)
	}
	var op db.KubernetesOperation
	rows, e := f.service.Store.Queries.ListKubernetesOperations(ctx, db.ListKubernetesOperationsParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, ClusterWide: true, NamespaceIds: []uuid.UUID{}, PageLimit: 10})
	if e != nil || len(rows) != 1 {
		t.Fatal(e, len(rows))
	}
	op = rows[0]
	var plan helmPlan
	if json.Unmarshal(op.PrivatePlan, &plan) != nil {
		t.Fatal("plan")
	}
	private, e := f.service.Store.Queries.GetKubernetesHelmInput(ctx, db.GetKubernetesHelmInputParams{EnterpriseID: f.actor.EnterpriseID, ID: uuid.MustParse(plan.Request.PrivateInputID)})
	if e != nil || private.Status != "bound" {
		t.Fatal(e)
	}
	for _, raw := range [][]byte{first.Diff, first.Preview, op.PrivatePlan, private.Envelope} {
		if bytes.Contains(raw, chart) || bytes.Contains(raw, []byte("protected-values-canary")) {
			t.Fatal("protected input leaked")
		}
	}
	changed := plan.Request
	changed.Name = "other-release"
	if _, e = f.service.Credentials.LoadHelmInput(ctx, f.actor.EnterpriseID, f.cluster, changed, true, "server", "test"); e == nil {
		t.Fatal("input moved between requests")
	}
	id := confirmHelm(f, first)
	if e = (Worker{Service: f.service}).execute(ctx, operationTask{f.actor.EnterpriseID, id}); e != nil {
		t.Fatal(e)
	}
	if f.operation(id).Status != "succeeded" || b.writes != 1 {
		t.Fatal("Helm approval did not complete", f.operation(id).ErrorCode)
	}
	if e = (Worker{Service: f.service}).execute(ctx, operationTask{f.actor.EnterpriseID, id}); e != nil || b.writes != 1 {
		t.Fatal("completed Helm replayed", e)
	}
}
func TestPostgresHelmChecksEveryRenderedObjectAndNamespaceUID(t *testing.T) {
	f, b, in := newHelmFixture(t)
	ctx := t.Context()
	d, _ := kubernetesinventory.Lookup("service")
	b.objects[0].Definition = d
	if _, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, []byte("chart"), nil, uuid.NewString()); !errors.Is(e, authorization.ErrAccessDenied) {
		t.Fatal("Helm permission replaced network permission", e)
	}
	d, _ = kubernetesinventory.Lookup("configmap")
	b.objects[0].Definition = d
	b.objects[0].NamespaceUID = "replacement"
	if _, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, []byte("chart"), nil, uuid.NewString()); e == nil {
		t.Fatal("recreated Namespace inherited permission")
	}
	b.objects[0].NamespaceUID = "orders-uid"
	p, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, []byte("chart"), nil, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	id := confirmHelm(f, p)
	row := f.operation(id)
	var plan helmPlan
	_ = json.Unmarshal(row.PrivatePlan, &plan)
	f.exec("UPDATE kubernetes_operations SET status='running' WHERE id=$1", id)
	if e = f.service.ValidateHelmWrite(ctx, f.actor.EnterpriseID, f.cluster, id, plan.Request, kuberneteshelm.Write{Version: "v1", Resource: "secrets", Namespace: "orders", Name: "sh.helm.release.v1.sample.v99", Verb: "PUT"}); e == nil {
		t.Fatal("unplanned history secret permitted")
	}
	f.exec("UPDATE kubernetes_namespace_resources SET namespace_uid='replacement' WHERE id=$1", f.namespace)
	if e = f.service.ValidateHelmDelivery(ctx, f.actor.EnterpriseID, f.cluster, id, plan.Request, true); e == nil {
		t.Fatal("changed Namespace UID remained authorized")
	}
}
func TestPostgresHelmRevocationStopsWritesAndUnknownIsNotReplayed(t *testing.T) {
	for _, mode := range []string{"revoke", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			f, b, in := newHelmFixture(t)
			p, e := f.service.PreviewHelm(t.Context(), f.actor, f.cluster, in, []byte("chart"), nil, uuid.NewString())
			if e != nil {
				t.Fatal(e)
			}
			id := confirmHelm(f, p)
			if mode == "revoke" {
				b.revoke = func() {
					f.exec("DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM role_bindings WHERE enterprise_id=$1 AND subject_id=$2) AND permission_id='kubernetes.helm.manage'", f.actor.EnterpriseID, f.actor.SubjectID)
				}
			} else {
				b.lost = true
			}
			w := Worker{Service: f.service}
			if e = w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); e != nil {
				t.Fatal(e)
			}
			want := "failed"
			if mode == "unknown" {
				want = "result_unknown"
			}
			if row := f.operation(id); row.Status != want {
				t.Fatal(row.Status, row.ErrorCode)
			}
			if e = w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); e != nil || b.writes != 1 {
				t.Fatal("write replayed", e, b.writes)
			}
		})
	}
}

func TestPostgresHelmReleaseLockFencesConcurrentAndUnknownOperations(t *testing.T) {
	f, _, in := newHelmFixture(t)
	ctx := t.Context()
	first, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, []byte("chart"), nil, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	second, e := f.service.PreviewHelm(ctx, f.actor, f.cluster, in, []byte("chart"), nil, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	id := confirmHelm(f, first)
	rows, e := f.service.Store.Queries.ListKubernetesOperations(ctx, db.ListKubernetesOperationsParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, ClusterWide: true, NamespaceIds: []uuid.UUID{}, PageLimit: 10})
	if e != nil {
		t.Fatal(e)
	}
	var secondRow db.KubernetesOperation
	for _, row := range rows {
		if row.PendingActionID.Valid && row.PendingActionID.UUID == second.ID {
			secondRow = row
		}
	}
	if secondRow.ID == uuid.Nil {
		t.Fatal("second preview not recorded")
	}
	commit := func() error {
		return f.service.Store.InTx(ctx, func(q *db.Queries) error {
			_, e := (ActionExtension{Service: f.service}).commitHelm(ctx, q, second, secondRow.PrivatePlan)
			return e
		})
	}
	if e = commit(); !errors.Is(e, resource.ErrActionInvalidated) {
		t.Fatal("concurrent release approval bypassed lock", e)
	}
	f.exec("UPDATE kubernetes_operations SET status='result_unknown',started_at=now() WHERE id=$1", id)
	if e = commit(); !errors.Is(e, resource.ErrActionInvalidated) {
		t.Fatal("unknown in-flight operation lost its fence", e)
	}
	f.exec("UPDATE kubernetes_operations SET started_at=now()-interval '7 minutes' WHERE id=$1", id)
	if e = commit(); e != nil {
		t.Fatal("explicit recovery could not acquire expired execution fence", e)
	}
	var plan helmPlan
	_ = json.Unmarshal(f.operation(id).PrivatePlan, &plan)
	if e = f.service.ValidateHelmWrite(ctx, f.actor.EnterpriseID, f.cluster, id, plan.Request, kuberneteshelm.Write{Version: "v1", Resource: "configmaps", Namespace: "orders", Name: "settings", Verb: "PATCH"}); e == nil {
		t.Fatal("old executor retained write authority after fence takeover")
	}
}
