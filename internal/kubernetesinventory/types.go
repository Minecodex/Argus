// Package kubernetesinventory shares the bounded Kubernetes object contract
// between direct readers and Connector execution. Authorization lives in the
// control plane; this package enforces the frozen scope and projects credentials
// before any object enters a command result, portal response or model context.
package kubernetesinventory

import (
	"encoding/json"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const SchemaVersion = "argus.kubernetes_inventory/v2"

var (
	ErrInvalid         = errors.New("KUBERNETES_INVENTORY_INVALID")
	ErrUnsupported     = errors.New("KUBERNETES_TYPE_UNSUPPORTED")
	ErrIdentityChanged = errors.New("KUBERNETES_OBJECT_IDENTITY_CHANGED")
	ErrExpired         = errors.New("KUBERNETES_RESOURCE_VERSION_EXPIRED")
	ErrForbidden       = errors.New("KUBERNETES_API_FORBIDDEN")
	ErrUnavailable     = errors.New("KUBERNETES_API_UNAVAILABLE")
	ErrBudget          = errors.New("KUBERNETES_RESULT_BUDGET_EXCEEDED")
	ErrProtocol        = errors.New("KUBERNETES_READER_VERSION_UNSUPPORTED")
)

type Definition struct {
	Key              string `json:"key"`
	Group            string `json:"group"`
	Version          string `json:"version"`
	Resource         string `json:"resource"`
	Kind             string `json:"kind"`
	Namespaced       bool   `json:"namespaced"`
	Category         string `json:"category"`
	ManagePermission string `json:"manage_permission"`
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	for _, known := range []error{ErrInvalid, ErrUnsupported, ErrIdentityChanged, ErrExpired, ErrForbidden, ErrUnavailable, ErrBudget, ErrProtocol, ErrMutationConflict, ErrMutationInvalid, ErrMutationUnknown, ErrDrainUnsafe, ErrCRDHasInstances} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrUnavailable.Error()
}
func ErrorFromCode(code string) error {
	if code == "" {
		return nil
	}
	for _, known := range []error{ErrInvalid, ErrUnsupported, ErrIdentityChanged, ErrExpired, ErrForbidden, ErrUnavailable, ErrBudget, ErrProtocol, ErrMutationConflict, ErrMutationInvalid, ErrMutationUnknown, ErrDrainUnsafe, ErrCRDHasInstances} {
		if code == known.Error() {
			return known
		}
	}
	return ErrUnavailable
}

func (d Definition) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: d.Group, Version: d.Version, Resource: d.Resource}
}

type Query struct {
	Type            string `json:"type"`
	Namespace       string `json:"namespace"`
	NamespaceUID    string `json:"namespace_uid"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	Labels          string `json:"labels"`
	Limit           int64  `json:"limit"`
	Continue        string `json:"continue"`
	ResourceVersion string `json:"resource_version"`
	AllNamespaces   bool   `json:"all_namespaces"`
}
type Object struct {
	Definition      Definition              `json:"definition"`
	UID             string                  `json:"uid"`
	Namespace       string                  `json:"namespace"`
	NamespaceUID    string                  `json:"namespace_uid,omitempty"`
	Name            string                  `json:"name"`
	ResourceVersion string                  `json:"resource_version"`
	Generation      int64                   `json:"generation"`
	Labels          map[string]string       `json:"labels"`
	Owners          []metav1.OwnerReference `json:"owners"`
	CreatedAt       time.Time               `json:"created_at"`
	ObservedAt      time.Time               `json:"observed_at"`
	Redacted        bool                    `json:"redacted"`
	Data            json.RawMessage         `json:"data"`
}
type Page struct {
	SchemaVersion   string    `json:"schema_version"`
	Items           []Object  `json:"items"`
	Continue        string    `json:"continue"`
	ResourceVersion string    `json:"resource_version"`
	Complete        bool      `json:"complete"`
	ObservedAt      time.Time `json:"observed_at"`
}
