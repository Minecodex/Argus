package kubernetesinventory

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

var crdResource = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

type TypeCatalog struct {
	Items             []Definition `json:"items"`
	Complete          bool         `json:"complete"`
	UnavailableReason string       `json:"unavailable_reason,omitempty"`
}

// Custom keys contain the CRD UID. Deleting and recreating a type with the same
// GVR never makes an old object operation or watch refer to the replacement.
func customDefinition(value unstructured.Unstructured, version string) (Definition, error) {
	group, _, _ := unstructured.NestedString(value.Object, "spec", "group")
	plural, _, _ := unstructured.NestedString(value.Object, "spec", "names", "plural")
	kind, _, _ := unstructured.NestedString(value.Object, "spec", "names", "kind")
	scope, _, _ := unstructured.NestedString(value.Object, "spec", "scope")
	if group == "" || plural == "" || kind == "" || value.GetUID() == "" || scope != "Namespaced" && scope != "Cluster" {
		return Definition{}, ErrUnsupported
	}
	versions, _, _ := unstructured.NestedSlice(value.Object, "spec", "versions")
	served := false
	for _, raw := range versions {
		entry, _ := raw.(map[string]any)
		if entry["name"] == version && entry["served"] == true {
			served = true
		}
	}
	if !served {
		return Definition{}, ErrUnsupported
	}
	return Definition{Key: "custom/" + group + "/" + version + "/" + plural + "@" + string(value.GetUID()), Group: group, Version: version, Resource: plural, Kind: kind, Namespaced: scope == "Namespaced", Category: "extension", ManagePermission: "kubernetes.custom_resource.manage"}, nil
}
func (c Client) Definition(ctx context.Context, key string) (Definition, error) {
	if value, ok := Lookup(key); ok {
		return value, nil
	}
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[0] != "custom" || len(key) > 1024 {
		return Definition{}, ErrUnsupported
	}
	plural, uid, ok := strings.Cut(parts[3], "@")
	if !ok || uid == "" || len(uid) > 128 || len(validation.IsDNS1123Subdomain(parts[1])) > 0 || len(validation.IsDNS1035Label(plural)) > 0 || len(validation.IsDNS1035Label(parts[2])) > 0 {
		return Definition{}, ErrInvalid
	}
	if c.Dynamic == nil {
		return Definition{}, ErrUnavailable
	}
	value, err := c.Dynamic.Resource(crdResource).Get(ctx, plural+"."+parts[1], metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Definition{}, ErrIdentityChanged
		}
		return Definition{}, apiError(err)
	}
	if string(value.GetUID()) != uid {
		return Definition{}, ErrIdentityChanged
	}
	definition, err := customDefinition(*value, parts[2])
	if err != nil {
		return Definition{}, err
	}
	if definition.Key != key {
		return Definition{}, ErrIdentityChanged
	}
	return definition, nil
}
func (c Client) Types(ctx context.Context) (TypeCatalog, error) {
	result := TypeCatalog{Items: Definitions(), Complete: true}
	if c.Dynamic == nil {
		return result, ErrUnavailable
	}
	continuation := ""
	for {
		list, err := c.Dynamic.Resource(crdResource).List(ctx, metav1.ListOptions{Limit: 20, Continue: continuation})
		if err != nil {
			result.Complete = false
			result.UnavailableReason = ErrorCode(apiError(err))
			return result, nil
		}
		for _, value := range list.Items {
			versions, _, _ := unstructured.NestedSlice(value.Object, "spec", "versions")
			for _, raw := range versions {
				entry, _ := raw.(map[string]any)
				name, _ := entry["name"].(string)
				if name == "" || entry["served"] != true {
					continue
				}
				definition, err := customDefinition(value, name)
				if err != nil {
					continue
				}
				if len(result.Items) >= 1000 {
					result.Complete = false
					result.UnavailableReason = ErrBudget.Error()
					return result, nil
				}
				result.Items = append(result.Items, definition)
			}
		}
		continuation = list.GetContinue()
		if continuation == "" {
			return result, nil
		}
	}
}
