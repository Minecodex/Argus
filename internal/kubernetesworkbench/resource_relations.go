package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sort"
)

// Relations only expose endpoints already included in the authorized snapshot.
// A reference in a projected manifest is not a grant to read its target.
func resourceRelations(objects []kubernetesinventory.Object) []ResourceRelation {
	byUID := map[string]kubernetesinventory.Object{}
	byName := map[string]string{}
	byReference := map[string]kubernetesinventory.Object{}
	result := []ResourceRelation{}
	seen := map[string]bool{}
	for _, obj := range objects {
		byUID[obj.UID] = obj
		byReference[obj.Definition.Key+"/"+obj.Namespace+"/"+obj.Name] = obj
		byName[obj.Definition.Key+"/"+obj.NamespaceUID+"/"+obj.Name] = obj.UID
	}
	add := func(from, to, kind string) {
		if _, ok := byUID[from]; !ok {
			return
		}
		if _, ok := byUID[to]; !ok {
			return
		}
		if from == to {
			return
		}
		key := from + "/" + to + "/" + kind
		if !seen[key] {
			result = append(result, ResourceRelation{from, to, kind})
			seen[key] = true
		}
	}
	for _, obj := range objects {
		for _, owner := range obj.Owners {
			if target, ok := byUID[string(owner.UID)]; ok && target.Definition.Kind == owner.Kind && target.Name == owner.Name && (!target.Definition.Namespaced || target.NamespaceUID == obj.NamespaceUID) {
				add(target.UID, obj.UID, "owns")
			}
		}
		var data map[string]any
		if json.Unmarshal(obj.Data, &data) != nil {
			continue
		}
		spec, _ := data["spec"].(map[string]any)
		key := obj.Definition.Key
		if key == "service" || key == "networkpolicy" || key == "pdb" {
			var selector labels.Selector
			if key == "service" {
				raw, _ := spec["selector"].(map[string]any)
				values := map[string]string{}
				for k, v := range raw {
					if s, ok := v.(string); ok {
						values[k] = s
					}
				}
				if len(values) > 0 {
					selector = labels.SelectorFromSet(values)
				}
			} else {
				selectorKey := "podSelector"
				if key == "pdb" {
					selectorKey = "selector"
				}
				raw, _ := json.Marshal(spec[selectorKey])
				var value metav1.LabelSelector
				if spec[selectorKey] != nil && json.Unmarshal(raw, &value) == nil {
					selector, _ = metav1.LabelSelectorAsSelector(&value)
				}
			}
			if selector != nil {
				for _, pod := range objects {
					if pod.Definition.Key == "pod" && pod.NamespaceUID == obj.NamespaceUID && selector.Matches(labels.Set(pod.Labels)) {
						add(obj.UID, pod.UID, "selects")
					}
					if key == "pdb" && pod.NamespaceUID == obj.NamespaceUID {
						var workload struct {
							Spec struct {
								Template struct {
									Metadata struct{ Labels map[string]string }
								}
							}
						}
						if json.Unmarshal(pod.Data, &workload) == nil && workload.Spec.Template.Metadata.Labels != nil && selector.Matches(labels.Set(workload.Spec.Template.Metadata.Labels)) {
							add(obj.UID, pod.UID, "selects")
						}
					}
				}
			}
		}
		if key == "rolebinding" || key == "clusterrolebinding" {
			ref, _ := data["roleRef"].(map[string]any)
			roleName, _ := ref["name"].(string)
			scope := obj.NamespaceUID
			roleType := "role"
			if ref["kind"] == "ClusterRole" {
				scope = ""
				roleType = "clusterrole"
			}
			add(obj.UID, byName[roleType+"/"+scope+"/"+roleName], "references")
			subjects, _ := data["subjects"].([]any)
			for _, raw := range subjects {
				subject, _ := raw.(map[string]any)
				if subject["kind"] != "ServiceAccount" {
					continue
				}
				ns, _ := subject["namespace"].(string)
				if ns == "" {
					ns = obj.Namespace
				}
				for _, target := range objects {
					if target.Definition.Key == "serviceaccount" && target.Namespace == ns && target.Name == subject["name"] {
						add(obj.UID, target.UID, "references")
					}
				}
			}
		}
		for _, ref := range objectReferences(obj) {
			target, ok := byReference[ref.Type+"/"+ref.Namespace+"/"+ref.Name]
			if !ok || target.Definition.Namespaced && obj.Definition.Namespaced && target.Namespace == obj.Namespace && target.NamespaceUID != obj.NamespaceUID {
				continue
			}
			relation := "references"
			if ref.Type == "pvc" {
				relation = "mounts"
			}
			if ref.Type == "storageclass" {
				relation = "uses"
			}
			add(obj.UID, target.UID, relation)
		}
		if key == "pod" {
			if node, ok := spec["nodeName"].(string); ok {
				add(byName["node//"+node], obj.UID, "schedules")
			}
		}
		if key == "endpointslice" {
			if service := obj.Labels["kubernetes.io/service-name"]; service != "" {
				add(byName["service/"+obj.NamespaceUID+"/"+service], obj.UID, "routes")
			}
			endpoints, _ := data["endpoints"].([]any)
			for _, raw := range endpoints {
				endpoint, _ := raw.(map[string]any)
				ref, _ := endpoint["targetRef"].(map[string]any)
				uid, _ := ref["uid"].(string)
				if target, ok := byUID[uid]; ok && target.NamespaceUID == obj.NamespaceUID && target.Definition.Kind == ref["kind"] && target.Name == ref["name"] {
					add(obj.UID, uid, "routes")
				}
			}
		}
		if key == "pvc" {
			if name, ok := spec["volumeName"].(string); ok {
				target := byUID[byName["pv//"+name]]
				var pv map[string]any
				_ = json.Unmarshal(target.Data, &pv)
				pvSpec, _ := pv["spec"].(map[string]any)
				claim, _ := pvSpec["claimRef"].(map[string]any)
				if claim["uid"] == obj.UID {
					add(obj.UID, target.UID, "binds")
				}
			}
			if name, ok := spec["storageClassName"].(string); ok {
				add(obj.UID, byName["storageclass//"+name], "uses")
			}
		}
		if key == "hpa" {
			target, _ := spec["scaleTargetRef"].(map[string]any)
			kind, _ := target["kind"].(string)
			name, _ := target["name"].(string)
			for _, item := range objects {
				if item.NamespaceUID == obj.NamespaceUID && item.Definition.Kind == kind && item.Name == name {
					add(obj.UID, item.UID, "scales")
				}
			}
		}
		if key == "ingress" {
			service := func(value any) {
				backend, _ := value.(map[string]any)
				svc, _ := backend["service"].(map[string]any)
				name, _ := svc["name"].(string)
				if name != "" {
					add(obj.UID, byName["service/"+obj.NamespaceUID+"/"+name], "routes")
				}
			}
			service(spec["defaultBackend"])
			rules, _ := spec["rules"].([]any)
			for _, raw := range rules {
				rule, _ := raw.(map[string]any)
				http, _ := rule["http"].(map[string]any)
				paths, _ := http["paths"].([]any)
				for _, raw := range paths {
					path, _ := raw.(map[string]any)
					service(path["backend"])
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { a, b := result[i], result[j]; return a.From+a.To+a.Kind < b.From+b.To+b.Kind })
	return result
}
