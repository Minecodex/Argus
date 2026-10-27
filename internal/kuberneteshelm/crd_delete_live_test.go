package kuberneteshelm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveHelmTemplateCRDDeletionRechecksDependencies(t *testing.T) {
	ctx, config, _, namespace := liveHelmEnvironment(t)
	api, err := dynamicAPI(config)
	if err != nil {
		t.Fatal(err)
	}
	owner := namespace.Name
	group := owner + ".test"
	name := "widgets." + group
	crds := api.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	instances := api.Resource(schema.GroupVersionResource{Group: group, Version: "v1", Resource: "widgets"}).Namespace(owner)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		child, e := instances.Get(cleanup, "retained", metav1.GetOptions{})
		if e == nil {
			if child.GetLabels()["argus.owner"] != owner {
				t.Error("owned Helm instance cleanup mismatch")
				return
			}
			uid := child.GetUID()
			if e = instances.Delete(cleanup, child.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
				t.Error(e)
				return
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error(e)
			return
		}
		current, e := crds.Get(cleanup, name, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("owned Helm CRD cleanup mismatch: %v", e)
			return
		}
		definition, _ := kubernetesinventory.Lookup("crd")
		if e = (kubernetesinventory.Client{Dynamic: api}).CheckDeletionDependencies(cleanup, definition, current); e != nil {
			t.Error("owned CRD dependencies were not cleared", e)
			return
		}
		uid := current.GetUID()
		if e = crds.Delete(cleanup, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
		}
	})
	manifest := fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %s
  labels:
    argus.owner: %s
spec:
  group: %s
  scope: Namespaced
  names:
    plural: widgets
    singular: widget
    kind: Plan8HelmDependencyWidget
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                value:
                  type: string
`, name, owner, group)
	permissions := []string{"kubernetes.crd.manage", "kubernetes.custom_resource.manage"}
	engine := Engine{Config: config, AuthorizeWrite: func(context.Context, Request, Write) error { return nil }}
	request := Request{SchemaVersion: SchemaVersion, APIServer: config.Host, OperationID: uuid.NewString(), Namespace: Namespace{Name: owner, UID: string(namespace.UID)}, Name: "dependency", Operation: "install", TimeoutSeconds: 90, Access: Access{ClusterWide: true, Namespaces: []Namespace{{Name: owner, UID: string(namespace.UID)}}, Permissions: permissions}}
	material := Material{Chart: packagedChart(t, "1.0.0", manifest), Key: bytes.Repeat([]byte{5}, 32)}
	preview, err := engine.Preview(ctx, request, material)
	if err != nil {
		t.Fatal("template CRD install preview", err)
	}
	request.SealedPlan = preview.SealedPlan
	installed, err := engine.Execute(ctx, request, material)
	if err != nil || !installed.Ready {
		t.Fatalf("template CRD actual install: %v ready=%v code=%s", err, installed.Ready, installed.ErrorCode)
	}
	crd, err := crds.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	create := func() {
		t.Helper()
		if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := instances.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": group + "/v1", "kind": "Plan8HelmDependencyWidget", "metadata": map[string]any{"name": "retained", "labels": map[string]any{"argus.owner": owner}}, "spec": map[string]any{"value": "must-survive"}}}, metav1.CreateOptions{})
			if apierrors.IsNotFound(e) {
				return false, nil
			}
			return e == nil, e
		}); err != nil {
			t.Fatal(err)
		}
	}
	create()
	request.Operation, request.OperationID, request.ExpectedRevision, request.SealedPlan = "uninstall", uuid.NewString(), 1, nil
	if _, err = engine.Preview(ctx, request, material); !errors.Is(err, kubernetesinventory.ErrCRDHasInstances) {
		t.Fatal("Helm preview did not explicitly block deleting a populated template CRD", err)
	}
	if err = instances.Delete(ctx, "retained", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	preview, err = engine.Preview(ctx, request, material)
	if err != nil {
		t.Fatal("empty template CRD uninstall preview", err)
	}
	create()
	request.SealedPlan = preview.SealedPlan
	result, err := engine.Execute(ctx, request, material)
	if err != nil && !errors.Is(err, kubernetesinventory.ErrCRDHasInstances) || err == nil && (result.Ready || result.ErrorCode != kubernetesinventory.ErrCRDHasInstances.Error()) {
		t.Fatalf("late dependency did not block actual Helm deletion: %v ready=%v code=%s", err, result.Ready, result.ErrorCode)
	}
	rejected := false
	for _, write := range result.Writes {
		rejected = rejected || write.Stage == "dependencies" && write.State == "rejected" && write.ErrorCode == kubernetesinventory.ErrCRDHasInstances.Error()
	}
	if !rejected {
		t.Fatal("Helm did not preserve the rejected dependency stage")
	}
	current, err := crds.Get(ctx, name, metav1.GetOptions{})
	if err != nil || current.GetUID() != crd.GetUID() {
		t.Fatal("late dependency lost the actual CRD", err)
	}
	if _, err = instances.Get(ctx, "retained", metav1.GetOptions{}); err != nil {
		t.Fatal("late dependency lost its instance", err)
	}
	t.Log("actual Helm template CRD install, populated preview rejection, late-instance per-write rejection and retained object identities passed; release metadata partial writes remain explicitly reported")
}
