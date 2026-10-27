package kubernetesworkbench

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/logcollection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestLogWorkloadUsesOwnerUIDsAndValidatesContainers(t *testing.T) {
	for _, pair := range [][2]string{{"replicaset", "deployment"}, {"job", "cronjob"}} {
		t.Run(pair[1], func(t *testing.T) {
			objects := map[string]kubernetesinventory.Object{}
			for _, kind := range []string{"pod", pair[0], pair[1]} {
				d, _ := kubernetesinventory.Lookup(kind)
				objects[kind] = kubernetesinventory.Object{Definition: d, UID: kind + "-uid", Name: kind, Namespace: "orders", NamespaceUID: "orders-uid", Data: []byte(fmt.Sprintf(`{"apiVersion":"v1","kind":%q,"metadata":{"name":%q,"uid":%q,"namespace":"orders"},"spec":{"containers":[{"name":"main"}],"initContainers":[{"name":"init"}]}}`, d.Kind, kind, kind+"-uid"))}
			}
			controller := true
			for i, kind := range []string{"pod", pair[0]} {
				parent := objects[pair[i]]
				v := objects[kind]
				v.Owners = []metav1.OwnerReference{{APIVersion: parent.Definition.Group + "/v1", Kind: parent.Definition.Kind, Name: parent.Name, UID: types.UID(parent.UID), Controller: &controller}}
				objects[kind] = v
			}
			get := func(_ context.Context, in ObjectInput) (kubernetesinventory.Object, error) {
				return objects[in.Type], nil
			}
			desired := logcollection.Workload{NamespaceID: uuid.New(), Namespace: "forged", NamespaceUID: "forged", Type: "pod", Name: "pod", UID: "pod-uid", Containers: []string{"main", "init"}, Ancestors: []string{"forged"}}
			result, err := resolveLogWorkload(t.Context(), desired, get)
			if err != nil || result.NamespaceUID != "orders-uid" || result.Namespace != "orders" || len(result.Ancestors) != 2 || result.Ancestors[1] != pair[1]+"-uid" {
				t.Fatalf("owner mapping: %+v %v", result, err)
			}
			root := logcollection.Workload{NamespaceID: result.NamespaceID, NamespaceUID: result.NamespaceUID, UID: pair[1] + "-uid", Containers: []string{"main"}}
			if !logcollection.WorkloadOverlap(result, root) {
				t.Fatal("Pod and controller claims did not intersect")
			}
			desired.Containers = []string{"missing"}
			if _, err = resolveLogWorkload(t.Context(), desired, get); err == nil {
				t.Fatal("unknown container accepted")
			}
			desired.Containers = []string{"main"}
			parent := objects[pair[0]]
			parent.UID = "replacement-uid"
			objects[pair[0]] = parent
			if _, err = resolveLogWorkload(t.Context(), desired, get); err == nil {
				t.Fatal("recreated owner accepted")
			}
		})
	}
}
