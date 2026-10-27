package kubernetesworkbench

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/logcollection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResolveLogWorkload uses the same UID-bound reads and Namespace authorization
// as the workbench. Client-provided namespace names and ancestors are ignored.
func (s Service) ResolveLogWorkload(ctx context.Context, a Actor, cluster uuid.UUID, desired logcollection.Workload) (logcollection.Workload, error) {
	return resolveLogWorkload(ctx, desired, func(ctx context.Context, in ObjectInput) (kubernetesinventory.Object, error) {
		return s.Get(ctx, a, cluster, in)
	})
}

func resolveLogWorkload(ctx context.Context, desired logcollection.Workload, get func(context.Context, ObjectInput) (kubernetesinventory.Object, error)) (logcollection.Workload, error) {
	result := logcollection.Workload{NamespaceID: desired.NamespaceID, Type: desired.Type, Name: desired.Name, UID: desired.UID, Containers: slices.Clone(desired.Containers)}
	if !logWorkloadType(result.Type) || result.NamespaceID == uuid.Nil || len(result.Containers) == 0 || len(result.Containers) > 32 {
		return result, kubernetesinventory.ErrInvalid
	}
	object, err := get(ctx, ObjectInput{NamespaceID: result.NamespaceID, Type: result.Type, Name: result.Name, UID: result.UID})
	if err != nil {
		return result, err
	}
	if object.NamespaceUID == "" || object.Namespace == "" || object.UID != result.UID || object.Name != result.Name || object.Definition.Key != result.Type {
		return result, kubernetesinventory.ErrIdentityChanged
	}
	result.Namespace, result.NamespaceUID = object.Namespace, object.NamespaceUID
	var data unstructured.Unstructured
	if data.UnmarshalJSON(object.Data) != nil {
		return result, kubernetesinventory.ErrInvalid
	}
	path := []string{"spec", "template", "spec"}
	if result.Type == "pod" {
		path = []string{"spec"}
	}
	if result.Type == "cronjob" {
		path = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	containers := map[string]bool{}
	for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
		values, _, e := unstructured.NestedSlice(data.Object, append(slices.Clone(path), field)...)
		if e != nil {
			return result, kubernetesinventory.ErrInvalid
		}
		for _, v := range values {
			if c, ok := v.(map[string]any); ok {
				if name, ok := c["name"].(string); ok {
					containers[name] = true
				}
			}
		}
	}
	for _, name := range result.Containers {
		if !containers[name] {
			return result, kubernetesinventory.ErrIdentityChanged
		}
	}
	slices.Sort(result.Containers)
	result.Containers = slices.Compact(result.Containers)
	seen := map[string]bool{object.UID: true}
	for depth := 0; depth < 4; depth++ {
		owners := slices.DeleteFunc(slices.Clone(object.Owners), func(o metav1.OwnerReference) bool { return o.Controller == nil || !*o.Controller })
		if len(owners) == 0 {
			return result, nil
		}
		if len(owners) != 1 {
			return result, kubernetesinventory.ErrInvalid
		}
		owner := owners[0]
		kind := strings.ToLower(owner.Kind)
		// A Pod owned by an unsupported CR can still be selected by its own UID.
		if !logWorkloadType(kind) {
			return result, nil
		}
		definition, _ := kubernetesinventory.Lookup(kind)
		if owner.APIVersion != definition.Group+"/"+definition.Version && !(definition.Group == "" && owner.APIVersion == definition.Version) {
			return result, kubernetesinventory.ErrIdentityChanged
		}
		uid := string(owner.UID)
		if uid == "" || seen[uid] {
			return result, kubernetesinventory.ErrIdentityChanged
		}
		seen[uid] = true
		parent, e := get(ctx, ObjectInput{NamespaceID: result.NamespaceID, Type: kind, Name: owner.Name, UID: uid})
		if e != nil {
			return result, e
		}
		if parent.NamespaceUID != result.NamespaceUID || parent.Namespace != result.Namespace || parent.UID != uid || parent.Name != owner.Name || parent.Definition != definition {
			return result, kubernetesinventory.ErrIdentityChanged
		}
		result.Ancestors = append(result.Ancestors, uid)
		object = parent
	}
	return result, kubernetesinventory.ErrBudget
}

func logWorkloadType(key string) bool {
	return slices.Contains([]string{"pod", "deployment", "replicaset", "daemonset", "statefulset", "job", "cronjob"}, key)
}
