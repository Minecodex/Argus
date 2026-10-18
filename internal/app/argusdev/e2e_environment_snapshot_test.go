package argusdev

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func formalSnapshotFixture(t *testing.T) (*E2EEnvironment, *fake.Clientset) {
	t.Helper()
	one := int32(1)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "formal", UID: "ns-original", Labels: map[string]string{"argus.io/release-id": "original", "app.kubernetes.io/part-of": "argus"}}}
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: ns.Name, UID: "worker-original", Generation: 1}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: ns.Name, UID: "claim-original"}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "volume", Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "volume", UID: "volume-original"}, Spec: corev1.PersistentVolumeSpec{ClaimRef: &corev1.ObjectReference{Namespace: ns.Name, Name: pvc.Name, UID: pvc.UID}, PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}}
	client := fake.NewClientset(ns, d, pvc, pv)
	env := &E2EEnvironment{ReleaseID: "test-run", Options: E2EOptions{Artifacts: t.TempDir(), RunID: "test-run"}, Kube: &E2EKube{Client: client}}
	return env, client
}

func TestFormalPausePersistsHealthAndStorageBeforeMutation(t *testing.T) {
	env, client := formalSnapshotFixture(t)
	patches := 0
	client.PrependReactor("patch", "deployments", func(ktesting.Action) (bool, kruntime.Object, error) {
		patches++
		raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "original-environment.json"))
		if err != nil {
			t.Fatal("mutation before baseline persisted", err)
		}
		var before formalEnvironmentSnapshot
		if err = json.Unmarshal(raw, &before); err != nil {
			t.Fatal(err)
		}
		if len(before.Workloads) != 1 || before.Workloads[0].Ready != 1 || before.Workloads[0].ObservedGeneration != 1 || before.Namespaces[0].PVCs[0].PVUID != "volume-original" {
			t.Fatalf("incomplete baseline: %+v", before)
		}
		return false, nil, nil
	})
	if err := (&App{}).pauseFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	if patches != 1 {
		t.Fatal("pause not executed")
	}
	if err := restoreFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "original-environment-restored.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Status string
		After  formalEnvironmentSnapshot
	}
	if err = json.Unmarshal(raw, &report); err != nil || report.Status != "passed" || report.After.Workloads[0].Ready != 1 {
		t.Fatalf("missing restoration proof: %s %v", raw, err)
	}
}

func TestFormalPauseSnapshotFailureLeavesReplicasUntouched(t *testing.T) {
	for _, failure := range []string{"pvc_list", "pv_get", "persist"} {
		t.Run(failure, func(t *testing.T) {
			env, client := formalSnapshotFixture(t)
			switch failure {
			case "pvc_list":
				client.PrependReactor("list", "persistentvolumeclaims", func(ktesting.Action) (bool, kruntime.Object, error) {
					return true, nil, errors.New("snapshot unavailable")
				})
			case "pv_get":
				client.PrependReactor("get", "persistentvolumes", func(ktesting.Action) (bool, kruntime.Object, error) {
					return true, nil, errors.New("snapshot unavailable")
				})
			case "persist":
				if err := os.Mkdir(filepath.Join(env.Options.Artifacts, "original-environment.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := (&App{}).pauseFormalWorkloads(t.Context(), env); err == nil {
				t.Fatal("incomplete snapshot permitted pause")
			}
			for _, action := range client.Actions() {
				if action.GetVerb() == "patch" {
					t.Fatal("snapshot failure mutated workload")
				}
			}
		})
	}
}

func TestFormalRestoreRejectsReplacedStorageAndRecordsFailure(t *testing.T) {
	for _, replacement := range []string{"pvc", "pv", "claim_ref"} {
		t.Run(replacement, func(t *testing.T) {
			env, client := formalSnapshotFixture(t)
			if err := (&App{}).pauseFormalWorkloads(t.Context(), env); err != nil {
				t.Fatal(err)
			}
			if replacement == "pvc" {
				p, _ := client.CoreV1().PersistentVolumeClaims("formal").Get(t.Context(), "data", metav1.GetOptions{})
				p.UID = "replacement"
				if _, err := client.CoreV1().PersistentVolumeClaims("formal").Update(t.Context(), p, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			} else {
				p, _ := client.CoreV1().PersistentVolumes().Get(t.Context(), "volume", metav1.GetOptions{})
				if replacement == "pv" {
					p.UID = "replacement"
				} else {
					p.Spec.ClaimRef.UID = "replacement"
				}
				if _, err := client.CoreV1().PersistentVolumes().Update(t.Context(), p, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			client.ClearActions()
			if err := restoreFormalWorkloads(t.Context(), env); err == nil {
				t.Fatal("replaced storage accepted")
			}
			for _, action := range client.Actions() {
				if action.GetVerb() == "patch" {
					t.Fatal("scaled workload onto replaced volume")
				}
			}
			raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "original-environment-restored.json"))
			if err != nil {
				t.Fatal(err)
			}
			var report struct{ Status string }
			if json.Unmarshal(raw, &report) != nil || report.Status != "failed" {
				t.Fatal("missing failed restoration evidence")
			}
		})
	}
}

func TestFormalRestorationDoesNotEquateReplicasWithHealth(t *testing.T) {
	env, client := formalSnapshotFixture(t)
	if err := (&App{}).pauseFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	d, _ := client.AppsV1().Deployments("formal").Get(t.Context(), "worker", metav1.GetOptions{})
	one := int32(1)
	d.Spec.Replicas = &one
	d.Status.ReadyReplicas = 0
	d.Status.AvailableReplicas = 0
	if _, err := client.AppsV1().Deployments("formal").Update(t.Context(), d, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	// The operator already restored replicas; final verification must still fail.
	env.Paused[0].Applied = false
	if err := restoreFormalWorkloads(t.Context(), env); err == nil {
		t.Fatal("unhealthy workload reported restored")
	}
}
