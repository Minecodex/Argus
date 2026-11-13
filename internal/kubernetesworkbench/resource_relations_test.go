package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func TestRelationsRequireAuthorizedExactIdentities(t *testing.T) {
	makeObject := func(kind, uid, name, ns, raw string) kubernetesinventory.Object {
		d, _ := kubernetesinventory.Lookup(kind)
		return kubernetesinventory.Object{Definition: d, UID: uid, Name: name, Namespace: ns, NamespaceUID: ns + "-uid", Data: json.RawMessage(raw)}
	}
	dep := makeObject("deployment", "old", "api", "one", `{}`)
	pod := makeObject("pod", "pod", "api-pod", "one", `{"spec":{"volumes":[{"persistentVolumeClaim":{"claimName":"data"}}]}}`)
	pod.Labels = map[string]string{"app": "api"}
	pod.Owners = []metav1.OwnerReference{{Kind: "Deployment", Name: "api", UID: types.UID("old")}}
	pvc := makeObject("pvc", "pvc", "data", "one", `{}`)
	svc := makeObject("service", "svc", "api", "one", `{"spec":{"selector":{"app":"api"}}}`)
	foreign := makeObject("pod", "foreign", "outside", "two", `{}`)
	foreign.Labels = pod.Labels
	edges := resourceRelations([]kubernetesinventory.Object{dep, pod, pvc, svc, foreign})
	if len(edges) != 3 {
		t.Fatal(edges)
	}
	for _, edge := range edges {
		if edge.To == "foreign" {
			t.Fatal("selector escaped namespace", edges)
		}
	}
	dep.UID = "replacement"
	edges = resourceRelations([]kubernetesinventory.Object{dep, pod, svc})
	if len(edges) != 1 || edges[0].Kind != "selects" {
		t.Fatal("same name substitution or invisible volume produced an edge", edges)
	}
}

func TestRBACRelationsExposeOnlyReturnedTargets(t *testing.T) {
	makeObject := func(kind, uid, name, ns string, raw string) kubernetesinventory.Object {
		d, _ := kubernetesinventory.Lookup(kind)
		nuid := ""
		if d.Namespaced {
			nuid = ns + "-uid"
		}
		return kubernetesinventory.Object{Definition: d, UID: uid, Name: name, Namespace: ns, NamespaceUID: nuid, Data: json.RawMessage(raw)}
	}
	role := makeObject("role", "role", "read", "orders", "{}")
	sa := makeObject("serviceaccount", "sa", "app", "orders", "{}")
	binding := makeObject("rolebinding", "binding", "grant", "orders", `{"roleRef":{"kind":"Role","name":"read"},"subjects":[{"kind":"ServiceAccount","name":"app","namespace":"orders"},{"kind":"ServiceAccount","name":"hidden","namespace":"outside"}]}`)
	edges := resourceRelations([]kubernetesinventory.Object{role, sa, binding})
	if len(edges) != 2 {
		t.Fatal(edges)
	}
	for _, edge := range edges {
		if edge.From != "binding" || edge.Kind != "references" || edge.To != "role" && edge.To != "sa" {
			t.Fatal(edge)
		}
	}
	edges = resourceRelations([]kubernetesinventory.Object{binding})
	if len(edges) != 0 {
		t.Fatal("a named reference granted target access", edges)
	}
}

func TestPDBRelationsMatchExpressionsAndWorkloadTemplates(t *testing.T) {
	makeObject := func(kind, uid, ns, raw string) kubernetesinventory.Object {
		d, _ := kubernetesinventory.Lookup(kind)
		return kubernetesinventory.Object{Definition: d, UID: uid, Name: uid, NamespaceUID: ns, Data: json.RawMessage(raw)}
	}
	pdb := makeObject("pdb", "budget", "orders", `{"spec":{"selector":{"matchExpressions":[{"key":"app","operator":"In","values":["api"]}]}}}`)
	pod := makeObject("pod", "pod", "orders", `{}`)
	pod.Labels = map[string]string{"app": "api"}
	deployment := makeObject("deployment", "api", "orders", `{"spec":{"template":{"metadata":{"labels":{"app":"api"}}}}}`)
	foreign := pod
	foreign.UID = "foreign"
	foreign.NamespaceUID = "other"
	edges := resourceRelations([]kubernetesinventory.Object{pdb, pod, deployment, foreign})
	if len(edges) != 2 {
		t.Fatal(edges)
	}
	for _, edge := range edges {
		if edge.From != "budget" || edge.To == "foreign" {
			t.Fatal(edges)
		}
	}
	pdb.Data = json.RawMessage(`{"spec":{}}`)
	if len(resourceRelations([]kubernetesinventory.Object{pdb, pod})) != 0 {
		t.Fatal("nil PDB selector matched pods")
	}
}

func TestExplicitReferencesAreBidirectionalAndIdentityBound(t *testing.T) {
	obj := func(kind, name, uid, ns, raw string) kubernetesinventory.Object {
		d, _ := kubernetesinventory.Lookup(kind)
		return kubernetesinventory.Object{Definition: d, Name: name, UID: uid, Namespace: ns, NamespaceUID: ns + "-uid", Data: json.RawMessage(raw)}
	}
	tests := []struct{ source, target kubernetesinventory.Object }{
		{obj("ingress", "api", "ing", "orders", `{"spec":{"tls":[{"secretName":"cert"}]}}`), obj("secret", "cert", "cert", "orders", `{}`)},
		{obj("pod", "api", "pod", "orders", `{"spec":{"volumes":[{"projected":{"sources":[{"configMap":{"name":"config"}},{"secret":{"name":"cert"}}]}}]}}`), obj("configmap", "config", "cm", "orders", `{}`)},
		{obj("statefulset", "db", "sts", "orders", `{"spec":{"serviceName":"db-headless"}}`), obj("service", "db-headless", "svc", "orders", `{}`)},
		{obj("deployment", "api", "dep", "orders", `{"spec":{"template":{"spec":{"containers":[{"envFrom":[{"configMapRef":{"name":"config"}}]}]}}}}`), obj("configmap", "config", "cm", "orders", `{}`)},
		{obj("rolebinding", "grant", "bind", "orders", `{"subjects":[{"kind":"ServiceAccount","name":"app","namespace":"shared"}]}`), obj("serviceaccount", "app", "sa", "shared", `{}`)},
	}
	for _, tc := range tests {
		if !isRelated(tc.source, tc.target, "relations") || !isRelated(tc.target, tc.source, "relations") {
			t.Fatalf("missing relation: %s %s", tc.source.Definition.Key, tc.target.Definition.Key)
		}
		if len(resourceRelations([]kubernetesinventory.Object{tc.source})) != 0 {
			t.Fatal("unreadable reference materialized")
		}
		if tc.source.Namespace == tc.target.Namespace {
			target := tc.target
			target.NamespaceUID = "replacement"
			if isRelated(tc.source, target, "relations") {
				t.Fatal("namespace replacement substituted")
			}
		}
	}
}
