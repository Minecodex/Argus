package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type formalEnvironmentSnapshot struct {
	CapturedAt time.Time                `json:"captured_at"`
	RunID      string                   `json:"run_id"`
	Workloads  []formalWorkloadHealth   `json:"workloads"`
	Namespaces []formalNamespaceStorage `json:"namespaces"`
}
type formalWorkloadHealth struct {
	Namespace          string            `json:"namespace"`
	Name               string            `json:"name"`
	Kind               string            `json:"kind"`
	UID                types.UID         `json:"uid"`
	Generation         int64             `json:"generation"`
	ObservedGeneration int64             `json:"observed_generation"`
	Desired            int32             `json:"desired"`
	Ready              int32             `json:"ready"`
	Available          int32             `json:"available"`
	Updated            int32             `json:"updated"`
	Conditions         map[string]string `json:"conditions"`
}
type formalNamespaceStorage struct {
	Name      string      `json:"name"`
	UID       types.UID   `json:"uid"`
	ReleaseID string      `json:"release_id"`
	PVCs      []formalPVC `json:"pvcs"`
}
type formalPVC struct {
	Name           string                               `json:"name"`
	UID            types.UID                            `json:"uid"`
	Phase          corev1.PersistentVolumeClaimPhase    `json:"phase"`
	StorageClass   string                               `json:"storage_class"`
	RequestedBytes int64                                `json:"requested_bytes"`
	CapacityBytes  int64                                `json:"capacity_bytes"`
	PV             string                               `json:"pv"`
	PVUID          types.UID                            `json:"pv_uid"`
	PVPhase        corev1.PersistentVolumePhase         `json:"pv_phase"`
	ClaimUID       types.UID                            `json:"claim_uid"`
	ReclaimPolicy  corev1.PersistentVolumeReclaimPolicy `json:"reclaim_policy"`
}

func captureFormalEnvironment(ctx context.Context, env *E2EEnvironment) (*formalEnvironmentSnapshot, error) {
	result := &formalEnvironmentSnapshot{CapturedAt: time.Now().UTC(), RunID: env.Options.RunID, Workloads: []formalWorkloadHealth{}, Namespaces: []formalNamespaceStorage{}}
	seen := map[string]bool{}
	for _, item := range env.Paused {
		if err := pausedNamespaceCurrent(ctx, env, item); err != nil {
			return nil, err
		}
		health, err := captureFormalHealth(ctx, env, item)
		if err != nil {
			return nil, err
		}
		result.Workloads = append(result.Workloads, health)
		if seen[item.Namespace] {
			continue
		}
		seen[item.Namespace] = true
		storage, err := captureFormalStorage(ctx, env, item.Namespace, item.NamespaceUID, item.ReleaseID)
		if err != nil {
			return nil, err
		}
		result.Namespaces = append(result.Namespaces, storage)
	}
	return result, nil
}

func captureFormalHealth(ctx context.Context, env *E2EEnvironment, item pausedWorkload) (formalWorkloadHealth, error) {
	h := formalWorkloadHealth{Namespace: item.Namespace, Name: item.Name, Kind: item.Kind, Conditions: map[string]string{}}
	if item.Kind == "Deployment" {
		d, err := env.Kube.Client.AppsV1().Deployments(item.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
		if err != nil {
			return h, err
		}
		if d.Spec.Replicas == nil {
			return h, fmt.Errorf("missing desired replicas %s/%s", item.Namespace, item.Name)
		}
		h.UID, h.Generation, h.ObservedGeneration = d.UID, d.Generation, d.Status.ObservedGeneration
		h.Desired, h.Ready, h.Available, h.Updated = *d.Spec.Replicas, d.Status.ReadyReplicas, d.Status.AvailableReplicas, d.Status.UpdatedReplicas
		for _, c := range d.Status.Conditions {
			h.Conditions[string(c.Type)] = string(c.Status)
		}
	} else {
		d, err := env.Kube.Client.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
		if err != nil {
			return h, err
		}
		if d.Spec.Replicas == nil {
			return h, fmt.Errorf("missing desired replicas %s/%s", item.Namespace, item.Name)
		}
		h.UID, h.Generation, h.ObservedGeneration = d.UID, d.Generation, d.Status.ObservedGeneration
		h.Desired, h.Ready, h.Available, h.Updated = *d.Spec.Replicas, d.Status.ReadyReplicas, d.Status.AvailableReplicas, d.Status.UpdatedReplicas
		for _, c := range d.Status.Conditions {
			h.Conditions[string(c.Type)] = string(c.Status)
		}
	}
	if h.UID != item.UID {
		return h, fmt.Errorf("workload replaced %s/%s", item.Namespace, item.Name)
	}
	return h, nil
}

func captureFormalStorage(ctx context.Context, env *E2EEnvironment, namespace string, uid types.UID, release string) (formalNamespaceStorage, error) {
	s := formalNamespaceStorage{Name: namespace, UID: uid, ReleaseID: release, PVCs: []formalPVC{}}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return s, err
	}
	if ns.UID != uid || ns.Labels["argus.io/release-id"] != release {
		return s, fmt.Errorf("storage namespace identity changed: %s", namespace)
	}
	claims, err := env.Kube.Client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return s, err
	}
	for _, c := range claims.Items {
		p := formalPVC{Name: c.Name, UID: c.UID, Phase: c.Status.Phase, PV: c.Spec.VolumeName, RequestedBytes: c.Spec.Resources.Requests.Storage().Value(), CapacityBytes: c.Status.Capacity.Storage().Value()}
		if c.Spec.StorageClassName != nil {
			p.StorageClass = *c.Spec.StorageClassName
		}
		if p.PV != "" {
			pv, err := env.Kube.Client.CoreV1().PersistentVolumes().Get(ctx, p.PV, metav1.GetOptions{})
			if err != nil {
				return s, err
			}
			p.PVUID, p.PVPhase, p.ReclaimPolicy = pv.UID, pv.Status.Phase, pv.Spec.PersistentVolumeReclaimPolicy
			if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID != c.UID || pv.Spec.ClaimRef.Namespace != namespace || pv.Spec.ClaimRef.Name != c.Name {
				return s, fmt.Errorf("PVC binding mismatch %s/%s", namespace, c.Name)
			}
			p.ClaimUID = pv.Spec.ClaimRef.UID
		}
		s.PVCs = append(s.PVCs, p)
	}
	slices.SortFunc(s.PVCs, func(a, b formalPVC) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return s, nil
}

func saveFormalSnapshot(env *E2EEnvironment, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, name), raw)
}

// Check before scaling workloads so a replaced volume is never silently used.
// A conflicting namespace does not prevent restoring independent namespaces.
func formalStorageConflicts(ctx context.Context, env *E2EEnvironment) map[string]error {
	failures := map[string]error{}
	if env.OriginalEnvironment == nil {
		return failures
	}
	for _, before := range env.OriginalEnvironment.Namespaces {
		after, err := captureFormalStorage(ctx, env, before.Name, before.UID, before.ReleaseID)
		if err == nil {
			err = compareFormalStorage(before, after)
		}
		if err != nil {
			failures[before.Name] = err
		}
	}
	return failures
}

func compareFormalStorage(before, after formalNamespaceStorage) error {
	// New unrelated PVCs may be created concurrently; never remove them.
	for _, old := range before.PVCs {
		i := slices.IndexFunc(after.PVCs, func(p formalPVC) bool { return p.Name == old.Name })
		if i < 0 {
			return fmt.Errorf("original PVC missing %s/%s", before.Name, old.Name)
		}
		now := after.PVCs[i]
		if now.UID != old.UID || now.PV != old.PV || now.PVUID != old.PVUID || now.ClaimUID != old.ClaimUID || now.StorageClass != old.StorageClass || now.ReclaimPolicy != old.ReclaimPolicy {
			return fmt.Errorf("original PVC/PV identity changed %s/%s", before.Name, old.Name)
		}
		if now.RequestedBytes < old.RequestedBytes || now.CapacityBytes < old.CapacityBytes || old.Phase == corev1.ClaimBound && now.Phase != corev1.ClaimBound || old.PVPhase == corev1.VolumeBound && now.PVPhase != corev1.VolumeBound {
			return fmt.Errorf("original PVC/PV degraded %s/%s", before.Name, old.Name)
		}
	}
	return nil
}

func verifyFormalRestoration(ctx context.Context, env *E2EEnvironment, restoreErr error) error {
	after, err := captureFormalEnvironment(ctx, env)
	failures := []error{restoreErr, err}
	if err == nil {
		for _, before := range env.OriginalEnvironment.Namespaces {
			i := slices.IndexFunc(after.Namespaces, func(n formalNamespaceStorage) bool { return n.Name == before.Name })
			if i < 0 {
				failures = append(failures, fmt.Errorf("missing original namespace %s", before.Name))
				continue
			}
			failures = append(failures, compareFormalStorage(before, after.Namespaces[i]))
		}
		for i, before := range env.OriginalEnvironment.Workloads {
			now := after.Workloads[i]
			if now.Desired != before.Desired || now.Ready < before.Ready || now.Available < before.Available || now.ObservedGeneration < now.Generation {
				failures = append(failures, fmt.Errorf("original workload health not restored %s/%s", before.Namespace, before.Name))
			}
		}
	}
	combined := errors.Join(failures...)
	status := "passed"
	message := ""
	if combined != nil {
		status = "failed"
		message = combined.Error()
	}
	report := map[string]any{"run_id": env.Options.RunID, "verified_at": time.Now().UTC(), "baseline": "original-environment.json", "status": status, "error": message, "after": after}
	return errors.Join(combined, saveFormalSnapshot(env, "original-environment-restored.json", report))
}
