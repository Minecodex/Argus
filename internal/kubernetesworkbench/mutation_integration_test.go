package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

type mutationFixtureBackend struct {
	inventoryFixtureBackend
	writes       int
	lostResponse bool
	observed     bool
	mutation     kubernetesinventory.Mutation
	previews     int
	previewError error
}

func (b *mutationFixtureBackend) outcome(input kubernetesinventory.Mutation) kubernetesinventory.MutationResult {
	d, _ := kubernetesinventory.Lookup(input.Query.Type)
	data, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": input.Query.Name, "namespace": input.Query.Namespace, "uid": "object-uid", "resourceVersion": "5", "annotations": map[string]string{"argus.io/operation-id": input.ID}}, "data": map[string]string{"value": "configured"}})
	return kubernetesinventory.MutationResult{After: &kubernetesinventory.Object{Definition: d, Name: input.Query.Name, Namespace: input.Query.Namespace, NamespaceUID: input.Query.NamespaceUID, UID: "object-uid", ResourceVersion: "5", Data: data}, Digest: "private-digest"}
}
func (b *mutationFixtureBackend) MutateInventory(_ context.Context, _ db.KubernetesCluster, in kubernetesinventory.Mutation, dry bool) (kubernetesinventory.MutationResult, error) {
	if dry {
		b.previews++
		if b.previewError != nil {
			return kubernetesinventory.MutationResult{}, b.previewError
		}
	}
	result := b.outcome(in)
	result.DryRun = dry
	result.Accepted = !dry
	if !dry {
		b.writes++
		b.mutation = in
		if b.lostResponse {
			return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrMutationUnknown
		}
	}
	return result, nil
}

func TestPostgresKubernetesPrivateInputsAndPreviewReplay(t *testing.T) {
	f := newMutationFixture(t)
	ctx := t.Context()
	key := uuid.NewString()
	input := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "secret", Operation: "create", Name: "settings", Object: json.RawMessage(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"settings","namespace":"orders"},"type":"Opaque"}`)}
	values := []byte(`{"password":"test-canary-never-persist"}`)
	first, err := f.service.PreviewMutationWithSecretValues(ctx, f.actor, input, values, key)
	if err != nil {
		t.Fatal(err)
	}
	f.backend.previewError = kubernetesinventory.ErrMutationConflict
	second, err := f.service.PreviewMutationWithSecretValues(ctx, f.actor, input, values, key)
	if err != nil || second.ID != first.ID || f.backend.previews != 1 {
		t.Fatalf("cached preview reran preflight: %v calls=%d", err, f.backend.previews)
	}
	if _, err = f.service.PreviewMutationWithSecretValues(ctx, f.actor, input, []byte(`{"password":"different"}`), key); !errors.Is(err, postgres.ErrIdempotencyConflict) {
		t.Fatalf("private value idempotency conflict: %v", err)
	}
	rows, err := f.service.Store.Queries.ListKubernetesOperations(ctx, db.ListKubernetesOperationsParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, ClusterWide: true, NamespaceIds: []uuid.UUID{}, PageLimit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("unexpected operations: %d %v", len(rows), err)
	}
	row := rows[0]
	var plan mutationPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil {
		t.Fatal("plan")
	}
	f.exec("UPDATE kubernetes_operations SET status='running',started_at=now(),heartbeat_at=now() WHERE id=$1", row.ID)
	load := func(in kubernetesinventory.Mutation) ([]byte, error) {
		return f.service.Credentials.LoadKubernetesInput(ctx, f.actor.EnterpriseID, f.cluster, in, false, "worker", f.cluster.String())
	}
	material, err := load(plan.Mutation)
	if err != nil || !bytes.Equal(material, values) {
		t.Fatal("approved private input unavailable")
	}
	clear(material)
	changed := plan.Mutation
	changed.Query.Name = "another-secret"
	if value, e := load(changed); e == nil {
		clear(value)
		t.Fatal("private input reused with changed target")
	}
	f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.actor.SubjectID)
	if value, e := load(plan.Mutation); e == nil {
		clear(value)
		t.Fatal("revoked private input was released")
	}
	f.exec("UPDATE kubernetes_operations SET status='cancelled' WHERE id=$1", row.ID)
	if err = f.service.Store.Queries.DeleteRetiredKubernetesPrivateInputs(ctx); err != nil {
		t.Fatal(err)
	}
	privateID, _ := uuid.Parse(plan.Mutation.PrivateInputID)
	if _, err = f.service.Store.Queries.GetKubernetesPrivateInput(ctx, db.GetKubernetesPrivateInputParams{ID: privateID, EnterpriseID: f.actor.EnterpriseID}); err == nil {
		t.Fatal("cancelled private input retained")
	}
}
func (b *mutationFixtureBackend) ObserveInventoryMutation(_ context.Context, _ db.KubernetesCluster, in kubernetesinventory.Mutation, _ string) (kubernetesinventory.MutationResult, error) {
	if !b.observed {
		return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrMutationUnknown
	}
	return b.outcome(in), nil
}

type mutationFixture struct {
	t                  *testing.T
	service            Service
	actor              Actor
	cluster, namespace uuid.UUID
	backend            *mutationFixtureBackend
}

func newMutationFixture(t *testing.T) *mutationFixture {
	t.Helper()
	// Worker.Reconcile scans the whole database. Unrelated fixtures must never
	// occupy its recovery batch or be reconciled through this controlled backend.
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "k8s_mutation", filepath.Join("..", "..", "migrations", "postgresql"))
	f := &mutationFixture{t: t, actor: Actor{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", AuthorizationVersion: 1}, cluster: uuid.New(), namespace: uuid.New(), backend: &mutationFixtureBackend{}}
	key := bytes.Repeat([]byte{8}, 32)
	f.service = Service{Store: store, Backend: f.backend, CursorKey: key, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}
	keyfile := filepath.Join(t.TempDir(), "keyring.json")
	keyJSON, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))}})
	if err := os.WriteFile(keyfile, keyJSON, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyfile)
	if err != nil {
		t.Fatal(err)
	}
	f.service.Credentials = secret.Service{Store: store, Keyring: keyring}
	department, role := uuid.New(), uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8 mutation',$2,'UTC')", f.actor.EnterpriseID, f.actor.EnterpriseID.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'operators')", department, f.actor.EnterpriseID)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'operator')", f.actor.SubjectID, f.actor.EnterpriseID, department, f.actor.SubjectID.String())
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'namespace writer')", role, f.actor.EnterpriseID)
	for key, description := range authorization.PermissionRegistry {
		f.exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", key, description, authorization.PermissionRegistryVersion)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions WHERE id IN ('kubernetes.read','kubernetes.configuration.manage','pending_action.confirm')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, role)
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'owned cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", f.cluster, f.actor.EnterpriseID)
	_, err = store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: f.namespace, EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: "orders", NamespaceUid: "orders-uid"})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, f.namespace)
	if err = audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: f.actor.EnterpriseID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *mutationFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.service.Store.Pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *mutationFixture) queue() (uuid.UUID, db.Execution) {
	f.t.Helper()
	input := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Operation: "create", Type: "configmap", Name: "settings", Object: json.RawMessage(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"settings","namespace":"orders"},"data":{"value":"configured"}}`)}
	return f.queueInput(input)
}
func (f *mutationFixture) queueInput(input MutationInput) (uuid.UUID, db.Execution) {
	return f.queuePrivateInput(input, nil)
}
func (f *mutationFixture) queuePrivateInput(input MutationInput, values []byte) (uuid.UUID, db.Execution) {
	f.t.Helper()
	ctx := f.t.Context()
	preview, err := f.service.PreviewMutationWithSecretValues(ctx, f.actor, input, values, uuid.NewString())
	if err != nil {
		f.t.Fatal(err)
	}
	if bytes.Contains(preview.Diff, []byte("argus.io/operation-id")) || bytes.Contains(preview.Diff, []byte("private-digest")) {
		f.t.Fatal("private evidence exposed in preview")
	}
	var publicLines []DiffLine
	if json.Unmarshal(preview.Diff, &publicLines) != nil || len(publicLines) == 0 {
		f.t.Fatal("preview does not satisfy public diff array contract")
	}
	x := ActionExtension{Service: f.service}
	r := resource.Service{Store: f.service.Store, Actions: f.service.Actions, Extension: x}
	workflow := actionservice.Service{Store: f.service.Store, Idempotency: f.service.Actions.Idempotency, Resources: r}
	confirmed, err := workflow.Confirm(ctx, f.actor.SubjectID.String(), uuid.NewString(), f.actor.EnterpriseID, f.actor.AuthorizationVersion, false, preview.ActionRef, uuid.NewString())
	if err != nil {
		f.t.Fatal(err)
	}
	if confirmed.Execution == nil {
		f.t.Fatal("execution not queued")
	}
	payload, _ := json.Marshal(actionservice.ExecutionTask{EnterpriseID: f.actor.EnterpriseID, ExecutionID: confirmed.Execution.ID})
	if err = (actionservice.Executor{Store: f.service.Store, Resources: r}).Handle(ctx, runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
		f.t.Fatal(err)
	}
	execution, err := f.service.Store.Queries.GetExecution(ctx, db.GetExecutionParams{ID: confirmed.Execution.ID, EnterpriseID: f.actor.EnterpriseID})
	if err != nil {
		f.t.Fatal(err)
	}
	if !execution.KubernetesOperationID.Valid || execution.Status != "result_unknown" {
		f.t.Fatalf("action completed before physical operation: %+v", execution)
	}
	return execution.KubernetesOperationID.UUID, execution
}
func (f *mutationFixture) operation(id uuid.UUID) db.KubernetesOperation {
	f.t.Helper()
	row, err := f.service.Store.Queries.GetKubernetesOperation(f.t.Context(), db.GetKubernetesOperationParams{EnterpriseID: f.actor.EnterpriseID, ID: id})
	if err != nil {
		f.t.Fatal(err)
	}
	return row
}

func TestPostgresKubernetesMutationAtMostOnceAndUnknownRecovery(t *testing.T) {
	f := newMutationFixture(t)
	id, execution := f.queue()
	w := Worker{Service: f.service}
	f.backend.lostResponse = true
	if err := w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if row := f.operation(id); row.Status != "result_unknown" {
		t.Fatalf("lost response marked complete: %s", row.Status)
	}
	if err := w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if f.backend.writes != 1 {
		t.Fatal("unknown write replayed")
	}
	f.exec("UPDATE kubernetes_operations SET heartbeat_at=now()-interval '1 minute',updated_at=now()-interval '1 minute' WHERE id=$1", id)
	f.backend.observed = true
	if err := w.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	row := f.operation(id)
	if row.Status != "succeeded" || f.backend.writes != 1 {
		t.Fatalf("observation did not settle: %s writes=%d", row.Status, f.backend.writes)
	}
	if bytes.Contains(row.Result, []byte("private-digest")) || bytes.Contains(row.Result, []byte("argus.io/operation-id")) {
		t.Fatal("private evidence in public result")
	}
	f.reconcileExecution(execution)
	page, err := f.service.ListOperations(t.Context(), f.actor, f.cluster, 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != id {
		t.Fatalf("scoped history: %+v %v", page, err)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.actor.EnterpriseID)
	if _, err = f.service.GetOperation(t.Context(), f.actor, f.cluster, id); err == nil {
		t.Fatal("operation history remained visible after scope revocation")
	}
}

func (f *mutationFixture) reconcileExecution(execution db.Execution) {
	f.reconcileExecutionStatus(execution, "succeeded")
}

func (f *mutationFixture) reconcileExecutionStatus(execution db.Execution, want string) {
	f.t.Helper()
	r := resource.Service{Store: f.service.Store, Actions: f.service.Actions, Extension: ActionExtension{Service: f.service}}
	payload, _ := json.Marshal(actionservice.ExecutionTask{EnterpriseID: f.actor.EnterpriseID, ExecutionID: execution.ID})
	if err := (actionservice.Executor{Store: f.service.Store, Resources: r}).Handle(f.t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
		f.t.Fatal(err)
	}
	final, err := f.service.Store.Queries.GetExecution(f.t.Context(), db.GetExecutionParams{ID: execution.ID, EnterpriseID: f.actor.EnterpriseID})
	if err != nil || final.Status != want {
		f.t.Fatalf("action not settled with operation: %s %v", final.Status, err)
	}
}
func TestPostgresKubernetesMutationRevokedBeforeDelivery(t *testing.T) {
	f := newMutationFixture(t)
	id, _ := f.queue()
	f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.actor.SubjectID)
	if err := (Worker{Service: f.service}).execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if row := f.operation(id); row.Status != "failed" || row.ErrorCode != "PERMISSION_DENIED" || f.backend.writes != 0 {
		t.Fatalf("revoked write delivered: %s %s writes=%d", row.Status, row.ErrorCode, f.backend.writes)
	}
}
func TestPostgresKubernetesMutationCrashBeforeWriteStaysUnknown(t *testing.T) {
	f := newMutationFixture(t)
	id, _ := f.queue()
	f.exec("UPDATE kubernetes_operations SET status='running',started_at=now(),heartbeat_at=now()-interval '1 minute',updated_at=now()-interval '1 minute' WHERE id=$1", id)
	if err := (Worker{Service: f.service}).Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if row := f.operation(id); row.Status != "result_unknown" || f.backend.writes != 0 {
		t.Fatalf("crashed write replayed: %s", row.Status)
	}
}
