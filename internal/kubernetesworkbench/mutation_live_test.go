package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
	"testing"
	"time"
)

type liveOperationBackend struct {
	client  kubernetesinventory.Client
	secrets secret.Service
}

func (b liveOperationBackend) ListInventory(ctx context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	return b.client.List(ctx, q)
}
func (b liveOperationBackend) GetInventory(ctx context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	return b.client.Get(ctx, q)
}
func (b liveOperationBackend) MutateInventory(ctx context.Context, cluster db.KubernetesCluster, in kubernetesinventory.Mutation, dry bool) (kubernetesinventory.MutationResult, error) {
	if in.PrivateInputID != "" {
		kind := "worker"
		if dry {
			kind = "server"
		}
		values, err := b.secrets.LoadKubernetesInput(ctx, cluster.EnterpriseID, cluster.ID, in, dry, kind, cluster.ID.String())
		if err != nil {
			return kubernetesinventory.MutationResult{}, err
		}
		defer clear(values)
		in, err = kubernetesinventory.WithSecretValues(in, values)
		if err != nil {
			return kubernetesinventory.MutationResult{}, err
		}
		defer clear(in.Object)
	}
	return b.client.Mutate(ctx, in, dry)
}
func (b liveOperationBackend) ObserveInventoryMutation(ctx context.Context, _ db.KubernetesCluster, in kubernetesinventory.Mutation, digest string) (kubernetesinventory.MutationResult, error) {
	return b.client.ReadMutationOutcome(ctx, in, digest)
}

// Exercises real PostgreSQL preview/confirm/worker/audit against real API objects.
// The injected API adapter is not an enrolled Connector or browser acceptance.
func TestLiveKubernetesOperationWorkflow(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("requires isolated Kubernetes context")
	}
	f := newMutationFixture(t)
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	owner := "plan8-workflow-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	nsAPI := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	ns, err := nsAPI.Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		current, e := nsAPI.Get(ctx, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetUID() != ns.GetUID() || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("cleanup ownership mismatch: %v", e)
			return
		}
		uid := ns.GetUID()
		if e = nsAPI.Delete(ctx, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := nsAPI.Get(ctx, owner, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				return true, nil
			}
			return false, e
		}); e != nil {
			t.Error(e)
		}
	})
	f.exec("UPDATE kubernetes_namespace_resources SET name=$1,namespace_uid=$2 WHERE id=$3", owner, string(ns.GetUID()), f.namespace)
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: client}, secrets: f.service.Credentials}
	manifest, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "settings", "namespace": owner}, "data": map[string]string{"mode": "initial"}})
	input := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "configmap", Name: "settings", Operation: "create", Object: manifest}
	objectAPI := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(owner)
	w := Worker{Service: f.service}
	for _, operation := range []string{"create", "patch", "delete"} {
		input.Operation = operation
		if operation != "create" {
			current, e := objectAPI.Get(t.Context(), "settings", metav1.GetOptions{})
			if e != nil {
				t.Fatal(e)
			}
			input.UID = string(current.GetUID())
			input.ExpectedVersion = current.GetResourceVersion()
			input.Object = nil
			if operation == "patch" {
				input.Object = json.RawMessage(`{"data":{"mode":"updated"}}`)
			}
		}
		id, execution := f.queueInput(input)
		if operation == "create" {
			if _, e := objectAPI.Get(t.Context(), "settings", metav1.GetOptions{}); !apierrors.IsNotFound(e) {
				t.Fatal("preview wrote a real object")
			}
		}
		if err = w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
			t.Fatal(err)
		}
		if err = wait.PollUntilContextTimeout(t.Context(), 200*time.Millisecond, 15*time.Second, true, func(ctx context.Context) (bool, error) {
			row := f.operation(id)
			if row.Status == "succeeded" {
				return true, nil
			}
			return false, w.observe(ctx, row)
		}); err != nil {
			t.Fatal(err)
		}
		f.reconcileExecution(execution)
		current, e := objectAPI.Get(t.Context(), "settings", metav1.GetOptions{})
		if operation == "delete" {
			if !apierrors.IsNotFound(e) {
				t.Fatal("delete completed before disappearance")
			}
		} else {
			if e != nil {
				t.Fatal(e)
			}
			mode, _, _ := unstructured.NestedString(current.Object, "data", "mode")
			if operation == "patch" && mode != "updated" {
				t.Fatalf("patch not applied: %s", mode)
			}
		}
	}
	runLiveAllocatedObjectWorkflows(t, f, owner)
	runLiveRollbackWorkflows(t, f, owner)
	runLiveCronJobWorkflow(t, f, owner)
	secretAPI := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "secrets"}).Namespace(owner)
	secretManifest, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "protected", "namespace": owner}, "type": "Opaque"})
	secretInput := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "secret", Name: "protected", Operation: "create", Object: secretManifest}
	canary := "plan8-private-canary-" + uuid.NewString()
	values, _ := json.Marshal(map[string]string{"password": canary})
	defer clear(values)
	secretID, execution := f.queuePrivateInput(secretInput, values)
	stored := f.operation(secretID)
	var privatePlan mutationPlan
	if json.Unmarshal(stored.PrivatePlan, &privatePlan) != nil {
		t.Fatal("invalid private plan")
	}
	if _, e := f.service.Credentials.LoadKubernetesInput(t.Context(), f.actor.EnterpriseID, f.cluster, privatePlan.Mutation, false, "worker", f.cluster.String()); e == nil {
		t.Fatal("private input leased before operation started")
	}
	if err = w.execute(t.Context(), operationTask{f.actor.EnterpriseID, secretID}); err != nil {
		t.Fatal(err)
	}
	if row := f.operation(secretID); row.Status != "succeeded" {
		t.Fatalf("secret operation status: %s, %s", row.Status, row.ErrorCode)
	}
	f.reconcileExecution(execution)
	actual, err := secretAPI.Get(t.Context(), "protected", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _, _ := unstructured.NestedString(actual.Object, "data", "password")
	decoded, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil || !bytes.Equal(decoded, []byte(canary)) {
		t.Fatal("protected value was not applied")
	}
	clear(decoded)
	projected, err := f.service.Backend.GetInventory(t.Context(), db.KubernetesCluster{}, kubernetesinventory.Query{Type: "secret", Namespace: owner, NamespaceUID: string(ns.GetUID()), Name: "protected", UID: string(actual.GetUID())})
	if err != nil || bytes.Contains(projected.Data, []byte(canary)) || !bytes.Contains(projected.Data, []byte("[REDACTED]")) {
		t.Fatal("public secret projection exposed or lost value mask")
	}
	var leaked bool
	err = f.service.Store.Pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pending_actions WHERE enterprise_id=$1 AND (preview::text||diff::text) LIKE '%'||$2||'%') OR EXISTS(SELECT 1 FROM pending_action_plans WHERE enterprise_id=$1 AND immutable_plan::text LIKE '%'||$2||'%') OR EXISTS(SELECT 1 FROM kubernetes_operations WHERE enterprise_id=$1 AND (private_plan::text||result::text) LIKE '%'||$2||'%') OR EXISTS(SELECT 1 FROM kubernetes_private_inputs WHERE enterprise_id=$1 AND envelope::text LIKE '%'||$2||'%') OR EXISTS(SELECT 1 FROM audit_events WHERE enterprise_id=$1 AND details::text LIKE '%'||$2||'%')`, f.actor.EnterpriseID, canary).Scan(&leaked)
	if err != nil || leaked {
		t.Fatalf("private input persistence leak=%v error=%v", leaked, err)
	}
	if err = f.service.Store.Queries.DeleteRetiredKubernetesPrivateInputs(t.Context()); err != nil {
		t.Fatal(err)
	}
	inputID, _ := uuid.Parse(privatePlan.Mutation.PrivateInputID)
	if _, err = f.service.Store.Queries.GetKubernetesPrivateInput(t.Context(), db.GetKubernetesPrivateInputParams{ID: inputID, EnterpriseID: f.actor.EnterpriseID}); err == nil {
		t.Fatal("terminal operation retained private input")
	}
}
