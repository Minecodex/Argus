// Package kubernetesmetrics joins authorized inventory identities with telemetry.
// Inventory lists remain independent of this bounded, read-only enrichment.
package kubernetesmetrics

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/telemetry"
	"time"
)

type Inventory interface {
	List(context.Context, kubernetesworkbench.Actor, uuid.UUID, kubernetesworkbench.ListInput) (kubernetesworkbench.Page, error)
	Get(context.Context, kubernetesworkbench.Actor, uuid.UUID, kubernetesworkbench.ObjectInput) (kubernetesinventory.Object, error)
	AccessScope(context.Context, kubernetesworkbench.Actor, uuid.UUID) (kubernetesworkbench.Scope, error)
}
type Service struct {
	Inventory Inventory
	Telemetry telemetry.Service
	Key       []byte
	Now       func() time.Time
}
type PrepareInput struct {
	List kubernetesworkbench.ListInput `json:"list"`
	UIDs []string                      `json:"uids"`
}
type Prepared struct {
	Token      string    `json:"token"`
	ObservedAt time.Time `json:"observed_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Complete   bool      `json:"complete"`
}
type ReadInput struct {
	Token  string `json:"token"`
	Metric string `json:"metric"`
}
type Point struct {
	Time     time.Time `json:"time"`
	Value    *float64  `json:"value"`
	Request  *float64  `json:"request,omitempty"`
	Limit    *float64  `json:"limit,omitempty"`
	Capacity *float64  `json:"capacity,omitempty"`
}
type Trend struct {
	Kind                 string   `json:"kind"`
	Unit                 string   `json:"unit"`
	State                string   `json:"state"`
	Reason               string   `json:"reason,omitempty"`
	Basis                string   `json:"basis"`
	Points               []Point  `json:"points"`
	CurrentRequest       *float64 `json:"current_request,omitempty"`
	CurrentLimit         *float64 `json:"current_limit,omitempty"`
	CurrentCapacity      *float64 `json:"current_capacity,omitempty"`
	Allocatable          *float64 `json:"allocatable,omitempty"`
	PartialConfiguration bool     `json:"partial_configuration"`
}
type Entry struct {
	UID    string  `json:"uid"`
	Trends []Trend `json:"trends"`
}
type Result struct {
	Items      []Entry   `json:"items"`
	ObservedAt time.Time `json:"observed_at"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Complete   bool      `json:"complete"`
}
type budget struct {
	Request *float64 `json:"r,omitempty"`
	Limit   *float64 `json:"l,omitempty"`
	Partial bool     `json:"p,omitempty"`
}
type row struct {
	UID          string             `json:"id"`
	Name         string             `json:"name"`
	Namespace    string             `json:"ns,omitempty"`
	NamespaceID  uuid.UUID          `json:"ns_id,omitempty"`
	NamespaceUID string             `json:"ns_uid,omitempty"`
	Created      time.Time          `json:"created"`
	Budgets      map[string]budget  `json:"budgets,omitempty"`
	Capacity     map[string]float64 `json:"capacity,omitempty"`
	Allocatable  map[string]float64 `json:"allocatable,omitempty"`
	Changed      bool               `json:"changed,omitempty"`
}
type pod struct {
	UID          string            `json:"id"`
	Name         string            `json:"name,omitempty"`
	Alias        string            `json:"alias,omitempty"`
	Namespace    string            `json:"ns"`
	NamespaceID  uuid.UUID         `json:"ns_id"`
	NamespaceUID string            `json:"ns_uid"`
	Created      time.Time         `json:"created"`
	Parents      []string          `json:"parents"`
	Containers   int               `json:"containers"`
	Terminal     bool              `json:"terminal,omitempty"`
	Budgets      map[string]budget `json:"budgets"`
	Volumes      []string          `json:"volumes,omitempty"`
}
type preparedState struct {
	Cluster     uuid.UUID `json:"cluster"`
	ClusterWide bool      `json:"cluster_wide"`
	Scope       string    `json:"scope"`
	Type        string    `json:"type"`
	Rows        []row     `json:"rows"`
	Pods        []pod     `json:"pods"`
	Complete    bool      `json:"complete"`
	PodError    string    `json:"pod_error,omitempty"`
	Observed    time.Time `json:"observed"`
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func Supported(kind string) bool {
	switch kind {
	case "namespace", "node", "pod", "deployment", "statefulset", "daemonset", "replicaset", "pvc":
		return true
	}
	return false
}
