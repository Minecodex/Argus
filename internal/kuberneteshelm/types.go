// Package kuberneteshelm supplies the shared, bounded native Helm engine.
// Domain authorization and private-input delivery belong to the control plane.
package kuberneteshelm

import (
	"context"
	"errors"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

const (
	SchemaVersion    = "argus.kubernetes_helm/v3"
	MaxArchiveBytes  = 384 << 10
	MaxExpandedBytes = 8 << 20
	MaxValuesBytes   = 96 << 10
	MaxManifestBytes = 512 << 10
	MaxObjects       = 100
	MaxCommandBytes  = 1 << 20
)

var (
	ErrInvalid           = errors.New("KUBERNETES_HELM_INVALID")
	ErrConflict          = errors.New("KUBERNETES_HELM_CONFLICT")
	ErrUnavailable       = errors.New("KUBERNETES_HELM_UNAVAILABLE")
	ErrDenied            = errors.New("KUBERNETES_HELM_FORBIDDEN")
	ErrBudget            = errors.New("KUBERNETES_HELM_BUDGET_EXCEEDED")
	ErrUnknown           = errors.New("KUBERNETES_HELM_OUTCOME_UNKNOWN")
	ErrSourceAuth        = errors.New("KUBERNETES_HELM_SOURCE_AUTH_FAILED")
	ErrSourceIntegrity   = errors.New("KUBERNETES_HELM_SOURCE_INTEGRITY_FAILED")
	ErrSourceUnavailable = errors.New("KUBERNETES_HELM_SOURCE_UNAVAILABLE")
)

type Namespace struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}
type Access struct {
	ClusterWide bool                               `json:"cluster_wide"`
	Namespaces  []Namespace                        `json:"namespaces"`
	Permissions []string                           `json:"permissions"`
	Policy      kubernetesinventory.MutationPolicy `json:"policy"`
}
type Request struct {
	Source           *ChartSource `json:"source,omitempty"`
	SchemaVersion    string       `json:"schema_version"`
	OperationID      string       `json:"operation_id"`
	APIServer        string       `json:"api_server"`
	Namespace        Namespace    `json:"namespace"`
	Name             string       `json:"name"`
	Operation        string       `json:"operation"`
	ExpectedRevision int          `json:"expected_revision"`
	RollbackRevision int          `json:"rollback_revision,omitempty"`
	TimeoutSeconds   int          `json:"timeout_seconds"`
	PrivateInputID   string       `json:"private_input_id,omitempty"`
	Access           Access       `json:"access"`
	SealedPlan       []byte       `json:"sealed_plan,omitempty"`
}
type Material struct {
	SourceCredentials *SourceCredentials `json:"source_credentials,omitempty"`
	Chart             []byte             `json:"chart,omitempty"`
	Values            []byte             `json:"values,omitempty"`
	Key               []byte             `json:"key"`
}
type Release struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	Chart        string    `json:"chart"`
	ChartVersion string    `json:"chart_version"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Object struct {
	Definition   kubernetesinventory.Definition `json:"definition"`
	Namespace    string                         `json:"namespace"`
	NamespaceUID string                         `json:"namespace_uid"`
	Name         string                         `json:"name"`
	UID          string                         `json:"uid"`
	Version      string                         `json:"resource_version"`
	Action       string                         `json:"action"`
	Hook         string                         `json:"hook,omitempty"`
	Keep         bool                           `json:"keep"`
	CRD          bool                           `json:"crd"`
	Validation   string                         `json:"validation"`
	Changes      []string                       `json:"changes"`
	Before       *kubernetesinventory.Object    `json:"before,omitempty"`
	After        *kubernetesinventory.Object    `json:"after,omitempty"`
}
type Result struct {
	EncryptedChart []byte           `json:"encrypted_chart,omitempty"`
	Source         *ChartProvenance `json:"source,omitempty"`
	SchemaVersion  string           `json:"schema_version"`
	Release        Release          `json:"release"`
	Objects        []Object         `json:"objects"`
	SealedPlan     []byte           `json:"sealed_plan,omitempty"`
	Accepted       bool             `json:"accepted"`
	DryRun         bool             `json:"dry_run"`
	Ready          bool             `json:"ready"`
	Applied        bool             `json:"applied"`
	ErrorCode      string           `json:"error_code,omitempty"`
	Writes         []WriteOutcome   `json:"writes,omitempty"`
}
type Write struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Verb      string `json:"verb"`
}
type WriteOutcome struct {
	Write     Write  `json:"target"`
	Stage     string `json:"stage"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
}
type Engine struct {
	SourcePolicy   ChartNetworkPolicy
	Config         *rest.Config
	AuthorizeWrite func(context.Context, Request, Write) error
}
type pin struct {
	Key     string                      `json:"key"`
	GVR     schema.GroupVersionResource `json:"gvr"`
	Object  Object                      `json:"object"`
	Desired *unstructured.Unstructured  `json:"desired,omitempty"`
	Digest  string                      `json:"digest,omitempty"`
}
type frozen struct {
	MaterialDigest string           `json:"material_digest,omitempty"`
	Source         *ChartProvenance `json:"source,omitempty"`
	SchemaVersion  string           `json:"schema_version"`
	Request        Request          `json:"request"`
	Marker         string           `json:"marker"`
	ChartName      string           `json:"chart_name"`
	ChartVersion   string           `json:"chart_version"`
	Manifest       string           `json:"manifest"`
	Hooks          []hook           `json:"hooks"`
	CRDs           []string         `json:"crds"`
	Pins           []pin            `json:"pins"`
	Storage        []storagePin     `json:"storage"`
}
type hook struct {
	Manifest string   `json:"manifest"`
	Name     string   `json:"name"`
	Events   []string `json:"events"`
}
type storagePin struct{ Name, UID, Version string }
