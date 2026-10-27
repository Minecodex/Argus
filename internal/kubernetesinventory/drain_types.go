package kubernetesinventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ErrDrainUnsafe = errors.New("KUBERNETES_DRAIN_UNSAFE")

const drainLimit = 200

var (
	drainNodes = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	drainPods  = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	drainPVCs  = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	drainPVs   = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}
	drainPDBs  = schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
)

// DrainPlan is a private, immutable allowlist. It is built by the execution
// adapter during preview, never accepted from a portal or model input.
type DrainPlan struct {
	Pods []DrainPod `json:"pods"`
}

type DrainPod struct {
	Namespace    string        `json:"namespace"`
	NamespaceUID string        `json:"namespace_uid"`
	Name         string        `json:"name"`
	UID          string        `json:"uid"`
	SpecDigest   string        `json:"spec_digest"`
	Disposition  string        `json:"disposition"`
	Volumes      []DrainVolume `json:"volumes"`
}

type DrainVolume struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Claim     string `json:"claim,omitempty"`
	ClaimUID  string `json:"claim_uid,omitempty"`
	Volume    string `json:"volume,omitempty"`
	VolumeUID string `json:"volume_uid,omitempty"`
	Local     bool   `json:"local"`
}

type DrainPodProgress struct {
	Namespace    string        `json:"namespace"`
	NamespaceUID string        `json:"namespace_uid"`
	Name         string        `json:"name"`
	UID          string        `json:"uid"`
	State        string        `json:"state"`
	Reason       string        `json:"reason,omitempty"`
	Volumes      []DrainVolume `json:"volumes"`
	Budgets      []DrainBudget `json:"budgets"`
}

type DrainBudget struct {
	Name               string `json:"name"`
	UID                string `json:"uid"`
	DisruptionsAllowed int64  `json:"disruptions_allowed"`
}

type DrainProgress struct {
	Phase string             `json:"phase"`
	Pods  []DrainPodProgress `json:"pods"`
}

func drainHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func drainPublicPod(p DrainPod) DrainPodProgress {
	return DrainPodProgress{Namespace: p.Namespace, NamespaceUID: p.NamespaceUID, Name: p.Name, UID: p.UID, State: "pending", Volumes: p.Volumes, Budgets: []DrainBudget{}}
}

func drainPhase(progress *DrainProgress) {
	progress.Phase = "complete"
	for _, p := range progress.Pods {
		if p.State == "blocked" {
			progress.Phase = "blocked"
			return
		}
		if p.State == "pending" || p.State == "terminating" || p.State == "eviction_requested" {
			progress.Phase = "draining"
		}
	}
}
