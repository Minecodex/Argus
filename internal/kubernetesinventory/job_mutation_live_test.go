package kubernetesinventory

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func TestLiveJobApprovalDigestSurvivesGeneratedIdentityAndCompletes(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("requires selected disposable Kubernetes context and cached alpine:3.21")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	owner := "plan8-job-" + uuid.NewString()[:8]
	namespaces := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	ns, err := namespaces.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 45*time.Second)
		defer c()
		current, e := namespaces.Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetUID() != ns.GetUID() || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("Job namespace ownership: %v", e)
			return
		}
		uid := current.GetUID()
		if e = namespaces.Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 300*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := namespaces.Get(ctx, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
	})
	manifest := map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": "run", "namespace": owner}, "spec": map[string]any{"backoffLimit": int64(0), "template": map[string]any{"spec": map[string]any{"restartPolicy": "Never", "terminationGracePeriodSeconds": int64(1), "containers": []any{map[string]any{"name": "main", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"/bin/sh", "-c", "sleep 2; echo plan8-job-complete"}}}}}}}
	raw, _ := json.Marshal(manifest)
	input := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: "job", Namespace: owner, NamespaceUID: string(ns.GetUID()), Name: "run"}, Object: raw}
	client := Client{Dynamic: api}
	first, err := client.Mutate(ctx, input, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Mutate(ctx, input, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == "" || first.Digest != second.Digest {
		t.Fatal("Job approval depends on a random dry-run UID")
	}
	input.ExpectedDigest = first.Digest
	actual, err := client.Mutate(ctx, input, false)
	if err != nil || !actual.Accepted || actual.After == nil {
		t.Fatalf("actual approved Job: %v", err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
		result, e := client.ReadMutationOutcome(ctx, input, first.Digest)
		if e != nil {
			return false, e
		}
		var job unstructured.Unstructured
		if job.UnmarshalJSON(result.After.Data) != nil {
			return false, ErrProtocol
		}
		conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
		for _, raw := range conditions {
			condition := raw.(map[string]any)
			if condition["type"] == "Complete" && condition["status"] == "True" {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Log("actual Job: repeat dry-run digests match, approved create succeeds, automatic UID selectors retained on the object, completion observed")
}
