package kuberneteshelm

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveHelmDeferredCRDAndNamespaceObjects(t *testing.T) {
	for _, kind := range []string{"namespace", "crd"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cfg, api, ns := liveHelmEnvironment(t)
			engine := Engine{Config: cfg, AuthorizeWrite: func(context.Context, Request, Write) error { return nil }}
			permissions := []string{"kubernetes.custom_resource.manage"}
			for _, d := range kubernetesinventory.Definitions() {
				if d.ManagePermission != "" {
					permissions = append(permissions, d.ManagePermission)
				}
			}
			request := Request{SchemaVersion: SchemaVersion, OperationID: uuid.NewString(), APIServer: cfg.Host, Namespace: Namespace{Name: ns.Name, UID: string(ns.UID)}, Name: "deferred", Operation: "install", TimeoutSeconds: 90, Access: Access{ClusterWide: true, Namespaces: []Namespace{{Name: ns.Name, UID: string(ns.UID)}}, Permissions: permissions}}
			files := map[string]string{"sample/Chart.yaml": "apiVersion: v2\nname: sample\nversion: 1.0.0\n"}
			if kind == "namespace" {
				child := ns.Name + "-child"
				files["sample/templates/objects.yaml"] = fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n  labels:\n    argus.owner: %s\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n  namespace: %s\ndata:\n  value: private-literal\n", child, ns.Name, child)
				t.Cleanup(func() {
					cleanup, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					defer cancel()
					current, e := api.CoreV1().Namespaces().Get(cleanup, child, metav1.GetOptions{})
					if apierrors.IsNotFound(e) {
						return
					}
					if e != nil || current.Labels["argus.owner"] != ns.Name {
						t.Error("child namespace ownership", e)
						return
					}
					if e = api.CoreV1().Namespaces().Delete(cleanup, child, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &current.UID}}); e != nil {
						t.Error(e)
						return
					}
					if e = wait.PollUntilContextCancel(cleanup, 300*time.Millisecond, true, func(ctx context.Context) (bool, error) {
						_, e := api.CoreV1().Namespaces().Get(ctx, child, metav1.GetOptions{})
						return apierrors.IsNotFound(e), nil
					}); e != nil {
						t.Error(e)
					}
				})
			} else {
				group := ns.Name + ".argus.test"
				name := "widgets." + group
				files["sample/crds/widgets.yaml"] = fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %s
  labels: {argus.owner: %s}
spec:
  group: %s
  scope: Namespaced
  names: {plural: widgets, singular: widget, kind: Widget}
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
                message: {type: string}
`, name, ns.Name, group)
				files["sample/templates/objects.yaml"] = fmt.Sprintf("apiVersion: %s/v1\nkind: Widget\nmetadata:\n  name: sample\n  namespace: %s\nspec:\n  message: private-literal\n", group, ns.Name)
				dynamic, e := dynamicAPI(cfg)
				if e != nil {
					t.Fatal(e)
				}
				crds := dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
				t.Cleanup(func() {
					cleanup, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					defer cancel()
					current, e := crds.Get(cleanup, name, metav1.GetOptions{})
					if apierrors.IsNotFound(e) {
						return
					}
					if e != nil || current.GetLabels()["argus.owner"] != ns.Name {
						t.Error("CRD ownership", e)
						return
					}
					uid := current.GetUID()
					if e = crds.Delete(cleanup, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
						t.Error(e)
						return
					}
					if e = wait.PollUntilContextCancel(cleanup, 300*time.Millisecond, true, func(ctx context.Context) (bool, error) {
						_, e := crds.Get(ctx, name, metav1.GetOptions{})
						return apierrors.IsNotFound(e), nil
					}); e != nil {
						t.Error(e)
					}
				})
			}
			material := Material{Chart: packagedFiles(t, files), Key: bytes.Repeat([]byte{7}, 32)}
			preview, e := engine.Preview(ctx, request, material)
			if e != nil {
				t.Fatal("deferred preview", e)
			}
			deferred := false
			for _, o := range preview.Objects {
				deferred = deferred || strings.HasPrefix(o.Validation, "deferred_")
			}
			if !deferred {
				t.Fatal("deferred server validation not declared")
			}
			request.SealedPlan = preview.SealedPlan
			result, e := engine.Execute(ctx, request, material)
			if e != nil || !result.Ready {
				t.Fatalf("deferred install: %v result=%+v", e, result)
			}
			request.Operation, request.OperationID, request.ExpectedRevision, request.SealedPlan = "uninstall", uuid.NewString(), 1, nil
			preview, e = engine.Preview(ctx, request, material)
			if e != nil {
				t.Fatal("deferred uninstall preview", e)
			}
			request.SealedPlan = preview.SealedPlan
			result, e = engine.Execute(ctx, request, material)
			if e != nil || !result.Ready {
				t.Fatalf("deferred uninstall: %v result=%+v", e, result)
			}
			if kind == "crd" {
				dynamic, e := dynamicAPI(cfg)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}).Get(ctx, "widgets."+ns.Name+".argus.test", metav1.GetOptions{}); e != nil {
					t.Fatal("Helm uninstall removed retained CRD", e)
				}
			}
		})
	}
}
