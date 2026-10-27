package kuberneteshelm

import (
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func publicObject(o Object) Object {
	before, after := map[string]any{}, map[string]any{}
	if o.Before != nil {
		_ = json.Unmarshal(o.Before.Data, &before)
	}
	if o.After != nil {
		_ = json.Unmarshal(o.After.Data, &after)
	}
	o.Changes = changedPaths(before, after, "")
	o.Before, o.After = nil, nil
	return o
}
func changedPaths(before, after any, prefix string) []string {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	a, aok := before.(map[string]any)
	b, bok := after.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range a {
			keys[k] = true
		}
		for k := range b {
			keys[k] = true
		}
		ordered := []string{}
		for k := range keys {
			ordered = append(ordered, k)
		}
		sort.Strings(ordered)
		out := []string{}
		for _, k := range ordered {
			path := prefix + "/" + k
			if path == "/data" || path == "/stringData" {
				if !reflect.DeepEqual(a[k], b[k]) {
					out = append(out, path+"/*")
				}
				continue
			}
			out = append(out, changedPaths(a[k], b[k], path)...)
		}
		return out
	}
	if prefix == "" {
		prefix = "/"
	}
	return []string{prefix}
}

func project(d kubernetesinventory.Definition, o unstructured.Unstructured, namespaceUID string) (kubernetesinventory.Object, error) {
	copy := o.DeepCopy()
	a := copy.GetAnnotations()
	delete(a, markerAnnotation)
	copy.SetAnnotations(a)
	value, e := kubernetesinventory.ProjectPreview(d, *copy, namespaceUID, time.Now())
	if e != nil {
		return value, e
	}
	// The marker is a private execution proof, not a user-supplied annotation.
	var document unstructured.Unstructured
	if json.Unmarshal(value.Data, &document.Object) == nil {
		a := document.GetAnnotations()
		delete(a, markerAnnotation)
		document.SetAnnotations(a)
		value.Data, _ = json.Marshal(document.Object)
	}
	return value, nil
}
