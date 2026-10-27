package kubernetesinventory

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Project(definition Definition, input unstructured.Unstructured, namespaceUID string, at time.Time) (Object, error) {
	return projectObject(definition, input, namespaceUID, at, true)
}

// ProjectPreview preserves absent server identities during dry-run rather than
// fabricating UIDs for objects which have not been created.
func ProjectPreview(definition Definition, input unstructured.Unstructured, namespaceUID string, at time.Time) (Object, error) {
	return projectObject(definition, input, namespaceUID, at, false)
}
func projectObject(definition Definition, input unstructured.Unstructured, namespaceUID string, at time.Time, requireIdentity bool) (Object, error) {
	if input.GetName() == "" || requireIdentity && (input.GetUID() == "" || input.GetResourceVersion() == "") {
		return Object{}, ErrInvalid
	}
	expectedVersion := definition.Version
	if definition.Group != "" {
		expectedVersion = definition.Group + "/" + definition.Version
	}
	if input.GetKind() != definition.Kind || input.GetAPIVersion() != expectedVersion {
		return Object{}, ErrInvalid
	}
	if definition.Namespaced != (input.GetNamespace() != "") {
		return Object{}, ErrIdentityChanged
	}
	value := input.DeepCopy()
	value.SetManagedFields(nil)
	annotations := value.GetAnnotations()
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	delete(annotations, operationAnnotation)
	value.SetAnnotations(annotations)
	redacted := false
	if definition.Key == "secret" {
		for _, field := range []string{"data", "stringData"} {
			if data, ok, _ := unstructured.NestedMap(value.Object, field); ok {
				for key := range data {
					data[key] = datapolicy.Marker
				}
				_ = unstructured.SetNestedMap(value.Object, data, field)
				redacted = true
			}
		}
	}
	redacted = redactEnvironment(value.Object) || redacted
	projected, changed, err := datapolicy.Project(value.Object)
	if err != nil {
		return Object{}, ErrInvalid
	}
	redacted = redacted || changed
	raw, err := json.Marshal(projected)
	if err != nil {
		return Object{}, ErrInvalid
	}
	var sanitized unstructured.Unstructured
	if sanitized.UnmarshalJSON(raw) != nil {
		return Object{}, ErrInvalid
	}
	labels := sanitized.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	owners := sanitized.GetOwnerReferences()
	if owners == nil {
		owners = []metav1.OwnerReference{}
	}
	return Object{Definition: definition, UID: string(input.GetUID()), Namespace: input.GetNamespace(), NamespaceUID: namespaceUID, Name: input.GetName(), ResourceVersion: input.GetResourceVersion(), Generation: input.GetGeneration(), Labels: labels, Owners: owners, CreatedAt: input.GetCreationTimestamp().Time, ObservedAt: at, Redacted: redacted, Data: raw}, nil
}
func redactEnvironment(value any) bool {
	changed := false
	switch current := value.(type) {
	case map[string]any:
		if name, ok := current["name"].(string); ok {
			for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == '.' }) {
				if datapolicy.CredentialKey(part) {
					if _, exists := current["value"]; exists {
						current["value"] = datapolicy.Marker
						changed = true
					}
					break
				}
			}
		}
		for _, item := range current {
			changed = redactEnvironment(item) || changed
		}
	case []any:
		for _, item := range current {
			changed = redactEnvironment(item) || changed
		}
	}
	return changed
}
