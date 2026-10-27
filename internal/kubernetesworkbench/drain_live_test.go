package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

type drainLiveBackend struct {
	liveOperationBackend
	loseOnce         bool
	begins, advances int
}

func (b *drainLiveBackend) MutateInventory(ctx context.Context, cluster db.KubernetesCluster, input kubernetesinventory.Mutation, dry bool) (kubernetesinventory.MutationResult, error) {
	result, err := b.liveOperationBackend.MutateInventory(ctx, cluster, input, dry)
	if !dry {
		b.begins++
		if err == nil && b.loseOnce {
			b.loseOnce = false
			return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrMutationUnknown
		}
	}
	return result, err
}
func (b *drainLiveBackend) AdvanceInventoryDrain(ctx context.Context, _ db.KubernetesCluster, input kubernetesinventory.Mutation) (kubernetesinventory.MutationResult, error) {
	b.advances++
	return b.client.AdvanceDrain(ctx, input)
}

// This test deliberately refuses normal clusters. It needs a disposable kind
// cluster, real worker/kubelet, and another schedulable node for replacement Pods.
func TestLiveNodeDrainApprovalPDBRecoveryRevocationAndLostResponse(t *testing.T) {
	name := os.Getenv("ARGUS_KUBERNETES_DRAIN_CONTEXT")
	if name == "" {
		t.Skip("requires explicitly owned disposable kind drain cluster")
	}
	if !strings.HasPrefix(name, "kind-plan8-drain-") {
		t.Fatal("refusing drain outside owned kind test context")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: name}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	nodes := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "nodes"})
	list, err := nodes.List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 {
		t.Fatal("requires exactly two dedicated nodes")
	}
	worker := ""
	for _, n := range list.Items {
		if !strings.HasPrefix(n.GetName(), "plan8-drain-") {
			t.Fatal("foreign node in disposable context")
		}
		if strings.HasSuffix(n.GetName(), "-worker") {
			worker = n.GetName()
		}
	}
	if worker == "" {
		t.Fatal("owned worker missing")
	}
	original, err := nodes.Get(ctx, worker, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wasCordoned, _, _ := unstructured.NestedBool(original.Object, "spec", "unschedulable")
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		current, e := nodes.Get(cleanup, worker, metav1.GetOptions{})
		if e != nil || current.GetUID() != original.GetUID() {
			t.Errorf("node restoration identity: %v", e)
			return
		}
		_ = unstructured.SetNestedField(current.Object, wasCordoned, "spec", "unschedulable")
		annotations := current.GetAnnotations()
		delete(annotations, "argus.io/operation-id")
		current.SetAnnotations(annotations)
		if _, e = nodes.Update(cleanup, current, metav1.UpdateOptions{}); e != nil {
			t.Error(e)
		}
	})
	owner := "plan8-drain-test-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	nsAPI := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	ns, err := nsAPI.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 40*time.Second)
		defer c()
		current, e := nsAPI.Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetUID() != ns.GetUID() || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("namespace cleanup identity: %v", e)
			return
		}
		uid := ns.GetUID()
		if e = nsAPI.Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 250*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := nsAPI.Get(ctx, owner, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				return true, nil
			}
			return false, e
		}); e != nil {
			t.Error(e)
		}
	})
	deployment := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "protected", "namespace": owner}, "spec": map[string]any{"replicas": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": owner}}, "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": owner}}, "spec": map[string]any{"automountServiceAccountToken": false, "terminationGracePeriodSeconds": int64(1), "affinity": map[string]any{"nodeAffinity": map[string]any{"preferredDuringSchedulingIgnoredDuringExecution": []any{map[string]any{"weight": int64(100), "preference": map[string]any{"matchExpressions": []any{map[string]any{"key": "kubernetes.io/hostname", "operator": "In", "values": []any{worker}}}}}}}}, "containers": []any{map[string]any{"name": "app", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"sleep", "600"}, "resources": map[string]any{"requests": map[string]any{"cpu": "5m", "memory": "8Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "24Mi"}}}}}}}}}
	_, err = api.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(owner).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pods := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(owner)
	var originalPodUID string
	err = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 45*time.Second, true, func(ctx context.Context) (bool, error) {
		page, e := pods.List(ctx, metav1.ListOptions{})
		if e != nil {
			return false, e
		}
		if len(page.Items) != 1 {
			return false, nil
		}
		p := page.Items[0]
		n, _, _ := unstructured.NestedString(p.Object, "spec", "nodeName")
		phase, _, _ := unstructured.NestedString(p.Object, "status", "phase")
		if n != worker {
			return false, nil
		}
		originalPodUID = string(p.GetUID())
		return phase == "Running", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pdbAPI := api.Resource(schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}).Namespace(owner)
	_, err = pdbAPI.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "policy/v1", "kind": "PodDisruptionBudget", "metadata": map[string]any{"name": "protect", "namespace": owner}, "spec": map[string]any{"minAvailable": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": owner}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		p, e := pdbAPI.Get(ctx, "protect", metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		observed, _, _ := unstructured.NestedInt64(p.Object, "status", "observedGeneration")
		return observed >= p.GetGeneration(), nil
	}); err != nil {
		t.Fatal(err)
	}
	f := newMutationFixture(t)
	backend := &drainLiveBackend{liveOperationBackend: liveOperationBackend{client: kubernetesinventory.Client{Dynamic: api}}, loseOnce: true}
	f.service.Backend = backend
	f.namespace = uuid.New()
	if _, err = f.service.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: f.namespace, EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: owner, NamespaceUid: string(ns.GetUID())}); err != nil {
		t.Fatal(err)
	}
	role := uuid.New()
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'node maintenance')", role, f.actor.EnterpriseID)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.node.manage')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, role)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, f.cluster)
	node, _ := nodes.Get(ctx, worker, metav1.GetOptions{})
	input := MutationInput{ClusterID: f.cluster, Operation: "drain", Type: "node", Name: worker, UID: string(node.GetUID()), ExpectedVersion: node.GetResourceVersion()}
	if _, err = f.service.PreviewMutation(ctx, f.actor, input, uuid.NewString()); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatalf("ordinary node permission allowed drain: %v", err)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.node.drain')", role)
	id, _ := f.queueInput(input)
	if current, _ := nodes.Get(ctx, worker, metav1.GetOptions{}); current.GetResourceVersion() != node.GetResourceVersion() {
		t.Fatal("preview changed node")
	}
	w := Worker{Service: f.service}
	if err = w.execute(ctx, operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if row := f.operation(id); row.Status != "result_unknown" {
		t.Fatalf("lost reply: %s %s", row.Status, row.ErrorCode)
	}
	if err = w.observe(ctx, f.operation(id)); err != nil {
		t.Fatal(err)
	}
	row := f.operation(id)
	if row.Status != "observing" || !bytes.Contains(row.Result, []byte(`"blocked"`)) || backend.begins != 1 {
		t.Fatalf("PDB recovery replayed node or hid blocker: %s %s begins=%d", row.Status, row.Result, backend.begins)
	}
	if bytes.Contains(row.Result, []byte("spec_digest")) || bytes.Contains(row.Result, []byte("argus.io/operation-id")) {
		t.Fatal("private drain evidence leaked")
	}
	before := backend.advances
	f.exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='kubernetes.node.drain'", role)
	if _, e := f.service.CancelDrain(ctx, f.actor, f.cluster, id, row.Version); !errors.Is(e, authorization.ErrAccessDenied) {
		t.Fatalf("cancel ignored revoked authority: %v", e)
	}
	if err = w.observe(ctx, row); err != nil {
		t.Fatal(err)
	}
	if f.operation(id).Status != "failed" || backend.advances != before {
		t.Fatal("revoked drain continued evicting")
	}
	remaining, _ := pods.List(ctx, metav1.ListOptions{})
	if len(remaining.Items) != 1 || string(remaining.Items[0].GetUID()) != originalPodUID {
		t.Fatal("PDB or revocation failed to preserve Pod")
	}
	t.Log("actual API dry-run had no node write; PDB denied eviction; lost result recovered without replay; grant-only revocation stopped continuation")
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.node.drain')", role)
	node, _ = nodes.Get(ctx, worker, metav1.GetOptions{})
	input.ExpectedVersion = node.GetResourceVersion()
	cancelID, cancelExecution := f.queueInput(input)
	if err = w.execute(ctx, operationTask{f.actor.EnterpriseID, cancelID}); err != nil {
		t.Fatal(err)
	}
	cancelRow := f.operation(cancelID)
	if _, err = f.service.CancelDrain(ctx, f.actor, f.cluster, cancelID, cancelRow.Version+1); !errors.Is(err, kubernetesinventory.ErrMutationConflict) {
		t.Fatalf("cancel ignored stale version: %v", err)
	}
	before = backend.advances
	if _, err = f.service.CancelDrain(ctx, f.actor, f.cluster, cancelID, cancelRow.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CancelDrain(ctx, f.actor, f.cluster, cancelID, cancelRow.Version); err != nil {
		t.Fatalf("cancel retry: %v", err)
	}
	if err = w.observe(ctx, f.operation(cancelID)); err != nil {
		t.Fatal(err)
	}
	if backend.advances != before || f.operation(cancelID).Status != "cancelled" {
		t.Fatal("cancelled drain dispatched another batch")
	}
	f.reconcileExecutionStatus(cancelExecution, "cancelled")
	t.Log("stale cancel rejected; cancellation stopped future batches, was idempotent, audited, and reconciled Execution without restoring scheduling")
	node, _ = nodes.Get(ctx, worker, metav1.GetOptions{})
	input.ExpectedVersion = node.GetResourceVersion()
	id, execution := f.queueInput(input)
	if err = w.execute(ctx, operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	pdb, err := pdbAPI.Get(ctx, "protect", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.workload.manage')", role)
	pdbID, pdbExecution := f.queueInput(MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "pdb", Name: "protect", UID: string(pdb.GetUID()), ExpectedVersion: pdb.GetResourceVersion(), Operation: "patch", Object: json.RawMessage(`{"spec":{"minAvailable":0}}`)})
	if err = w.execute(ctx, operationTask{f.actor.EnterpriseID, pdbID}); err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		row := f.operation(pdbID)
		if row.Status == "failed" {
			t.Fatalf("PDB workflow: %s", row.ErrorCode)
		}
		if row.Status == "succeeded" {
			return true, nil
		}
		return false, w.observe(ctx, row)
	}); err != nil {
		t.Fatal(err)
	}
	f.reconcileExecution(pdbExecution)
	t.Log("PDB policy changed through the same approved object workflow and waited for the native controller's observedGeneration")
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 45*time.Second, true, func(ctx context.Context) (bool, error) {
		row := f.operation(id)
		if row.Status == "failed" {
			t.Fatalf("drain failed: %s %s", row.ErrorCode, row.Result)
		}
		if row.Status == "succeeded" {
			return true, nil
		}
		return false, w.observe(ctx, row)
	}); err != nil {
		t.Fatal(err)
	}
	f.reconcileExecution(execution)
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
		page, e := pods.List(ctx, metav1.ListOptions{})
		if e != nil {
			return false, e
		}
		if len(page.Items) != 1 {
			return false, nil
		}
		p := page.Items[0]
		assigned, _, _ := unstructured.NestedString(p.Object, "spec", "nodeName")
		phase, _, _ := unstructured.NestedString(p.Object, "status", "phase")
		return string(p.GetUID()) != originalPodUID && assigned != worker && phase == "Running", nil
	}); err != nil {
		t.Fatal(err)
	}
	node, _ = nodes.Get(ctx, worker, metav1.GetOptions{})
	cordoned, _, _ := unstructured.NestedBool(node.Object, "spec", "unschedulable")
	if !cordoned {
		t.Fatal("drain silently uncordoned node")
	}
	t.Log("PDB change allowed actual UID-bound eviction; old Pod disappeared; replacement ran on the other node; operation and Execution converged; node remains cordoned")
	var result map[string]any
	if json.Unmarshal(f.operation(id).Result, &result) != nil {
		t.Fatal("invalid persisted result")
	}
}
