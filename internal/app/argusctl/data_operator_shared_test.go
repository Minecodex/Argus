package argusctl

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSharedDataDefinitionsAreReadOnlyAndBindingsRemainIsolated(t *testing.T) {
	crd := `{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"fixtures.kafka.strimzi.io"},"spec":{"group":"kafka.strimzi.io","scope":"Namespaced","names":{"plural":"fixtures","kind":"Fixture"},"versions":[{"name":"v1","served":true,"storage":true,"schema":{"openAPIV3Schema":{"type":"object"}}}]},"status":{"conditions":[{"type":"Established","status":"True"}]}}`
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "operator", Version: "1.1.0"}, Values: map[string]any{}, Files: []*common.File{{Name: "crds/fixture.yaml", Data: []byte(crd)}}, Templates: []*common.File{{Name: "templates/roles.yaml", Data: []byte(`{{- if not (hasKey .Values "createGlobalResources") }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: strimzi-cluster-operator-namespaced}
rules: [{apiGroups: [""], resources: [pods], verbs: [get]}]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: strimzi-cluster-operator}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: strimzi-cluster-operator-namespaced}
subjects: [{kind: ServiceAccount, name: operator, namespace: "{{ .Release.Namespace }}"}]
{{- end }}`)}}}
	var live unstructured.Unstructured
	if err := live.UnmarshalJSON([]byte(crd)); err != nil {
		t.Fatal(err)
	}
	live.SetLabels(map[string]string{"argus.io/owner-release": "formal"})
	_ = unstructured.SetNestedField(live.Object, "FixtureList", "spec", "names", "listKind")
	_ = unstructured.SetNestedField(live.Object, "fixture", "spec", "names", "singular")
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "strimzi-cluster-operator-namespaced", Annotations: map[string]string{"meta.helm.sh/release-name": "formal-st", "meta.helm.sh/release-namespace": "formal"}}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}}
	typed := fake.NewClientset(role)
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), &live)
	clients := &kubeClients{typed: typed, dynamic: dynamic}
	cfg := &InstallConfig{}
	cfg.Spec.ReleaseID = "temporary"
	cfg.Spec.Namespaces.Observability = "temporary"
	values := map[string]any{}
	owner, err := configureSharedStrimzi(context.Background(), cfg, clients, ch, values)
	if err != nil || owner != "formal/formal-st" || values["createGlobalResources"] != false || !cfg.sharedDataCRDs[live.GetName()] {
		t.Fatalf("share failed: %s %v", owner, err)
	}
	objects, err := operatorObjects(ch, cfg, values)
	if err != nil || len(objects) != 1 || objects[0].GetKind() != "ClusterRoleBinding" || objects[0].GetName() == "strimzi-cluster-operator" {
		t.Fatalf("binding isolation failed: %v %+v", err, objects)
	}
	for _, action := range append(typed.Actions(), dynamic.Actions()...) {
		if action.GetVerb() != "get" {
			t.Fatalf("foreign object changed: %v", action)
		}
	}
	role.Rules[0].Verbs = append(role.Rules[0].Verbs, "delete")
	if _, err = typed.RbacV1().ClusterRoles().Update(context.Background(), role, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = configureSharedStrimzi(context.Background(), cfg, clients, ch, map[string]any{}); err == nil {
		t.Fatal("broader foreign role accepted")
	}
}

func TestSharedStrimziLiveCompatibility(t *testing.T) {
	contextName := os.Getenv("ARGUS_SHARED_DATA_TEST_CONTEXT")
	if contextName == "" {
		t.Skip("read-only live context not configured")
	}
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := CompatibleStrimziOwner(context.Background(), contextName, root)
	if err != nil || owner == "" {
		t.Fatalf("live definitions not reusable: %s %v", owner, err)
	}
	clients, err := clientsFor(contextName)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := (helmManager{cacheDir: filepath.Join(root, "deploy", ".cache", "charts"), log: io.Discard}).loadRemoteChart(context.Background(), "altinity-0.27.3", altinityChartURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &InstallConfig{}
	cfg.Spec.ReleaseID = "argus-compatibility-check"
	if shared, err := configureSharedDataCRDs(context.Background(), cfg, clients, ch); err != nil || !shared {
		t.Fatalf("live ClickHouse definitions: %v shared=%v", err, shared)
	}
}
