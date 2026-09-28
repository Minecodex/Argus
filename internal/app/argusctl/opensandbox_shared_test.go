package argusctl

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSharedOpenSandboxIsReadOnlyAndRejectsSchemaDrift(t *testing.T) {
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "opensandbox-controller", Version: "0.2.0", APIVersion: "v2"}}
	objects := []runtime.Object{}
	for _, name := range []string{"batchsandboxes", "pools", "sandboxsnapshots"} {
		object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": map[string]any{"name": name + ".sandbox.opensandbox.io"}, "spec": map[string]any{"group": "sandbox.opensandbox.io", "scope": "Namespaced", "names": map[string]any{"plural": name, "kind": name}, "versions": []any{map[string]any{"name": "v1alpha1", "served": true, "storage": true, "schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object"}}}}}}}
		raw, _ := json.Marshal(object.Object)
		ch.Templates = append(ch.Templates, &common.File{Name: "templates/" + name + ".yaml", Data: raw})
		object.SetAnnotations(map[string]string{"meta.helm.sh/release-name": "external", "meta.helm.sh/release-namespace": "other-project"})
		object.Object["status"] = map[string]any{"conditions": []any{map[string]any{"type": "Established", "status": "True"}}}
		objects = append(objects, object)
	}
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opensandbox-controller-manager", Namespace: "other-project", Generation: 1, Labels: map[string]string{"app.kubernetes.io/instance": "external", "app.kubernetes.io/version": "0.2.0"}}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "manager", Image: "mirror.example/opensandbox/controller:v0.2.0"}}}}}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: 1}}
	typed := fake.NewClientset(deployment)
	clients := &kubeClients{typed: typed, dynamic: dynamic}
	cfg := &InstallConfig{}
	cfg.Spec.ReleaseID = "own"
	cfg.Spec.Namespaces.Sandbox = "own-sandbox"
	shared, err := sharedOpenSandboxController(context.Background(), cfg, clients, ch)
	if err != nil || shared != "other-project/opensandbox-controller-manager" {
		t.Fatalf("compatible controller not reused: %s %v", shared, err)
	}
	for _, action := range append(dynamic.Actions(), typed.Actions()...) {
		if action.GetVerb() != "get" {
			t.Fatalf("shared dependency was mutated: %s", action.GetVerb())
		}
	}
	gvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	live, _ := dynamic.Resource(gvr).Get(context.Background(), "pools.sandbox.opensandbox.io", metav1.GetOptions{})
	_ = unstructured.SetNestedField(live.Object, "Cluster", "spec", "scope")
	_, _ = dynamic.Resource(gvr).Update(context.Background(), live, metav1.UpdateOptions{})
	if _, err = sharedOpenSandboxController(context.Background(), cfg, clients, ch); err == nil {
		t.Fatal("incompatible cluster-scoped schema accepted")
	}
}

// This opt-in live check performs only GETs; it never adopts external CRDs.
func TestSharedOpenSandboxLiveCompatibility(t *testing.T) {
	contextName := os.Getenv("ARGUS_SHARED_OPENSANDBOX_TEST_CONTEXT")
	if contextName == "" {
		t.Skip("read-only live context not configured")
	}
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "evaluation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.ReleaseID = "argus-compatibility-check"
	cfg.Spec.Namespaces.Sandbox = "argus-compatibility-check"
	clients, err := clientsFor(contextName)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := (helmManager{cacheDir: filepath.Join(root, "deploy", ".cache", "charts"), log: io.Discard}).loadOpenSandboxControllerChart(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shared, err := sharedOpenSandboxController(context.Background(), cfg, clients, ch)
	if err != nil {
		expected, _ := openSandboxCRDs(ch, openSandboxControllerValues(cfg))
		actual := map[string]*unstructured.Unstructured{}
		for name := range expected {
			v, e := clients.dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}).Get(context.Background(), name, metav1.GetOptions{})
			if e == nil {
				actual[name] = v
			}
		}
		dir := filepath.Join(root, "artifacts", "opensandbox-compatibility")
		_ = os.MkdirAll(dir, 0700)
		for name, value := range map[string]any{"expected": expected, "actual": actual} {
			raw, _ := json.MarshalIndent(value, "", "  ")
			_ = os.WriteFile(filepath.Join(dir, name+".json"), raw, 0600)
		}
		t.Fatal(err)
	}
	if shared == "" {
		t.Fatal("no external controller discovered")
	}
	t.Log("compatible external controller:", shared)
}

func TestSharedSandboxSchemaAllowsOnlyNonBreakingAdditions(t *testing.T) {
	wanted := map[string]any{"type": "object", "properties": map[string]any{"existing": map[string]any{"type": "string", "description": "old docs"}}}
	actual := map[string]any{"type": "object", "properties": map[string]any{"existing": map[string]any{"type": "string", "description": "new docs"}, "optional": map[string]any{"type": "string"}}}
	if !compatibleSandboxSchema(wanted, actual) {
		t.Fatal("optional additive field rejected")
	}
	actual["required"] = []any{"optional"}
	if compatibleSandboxSchema(wanted, actual) {
		t.Fatal("new required field accepted")
	}
	delete(actual, "required")
	actual["properties"].(map[string]any)["optional"].(map[string]any)["default"] = "changes behavior"
	if compatibleSandboxSchema(wanted, actual) {
		t.Fatal("new implicit default accepted")
	}
}
