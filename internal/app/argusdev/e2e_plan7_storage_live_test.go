package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPlan7LiveWorkspaceStorageBeyondEightVolumes(t *testing.T) {
	if os.Getenv("ARGUS_P7_STORAGE_LIVE") != "1" {
		t.Skip("requires explicit local storage acceptance")
	}
	artifacts := os.Getenv("ARGUS_P7_STORAGE_ARTIFACTS")
	if artifacts == "" {
		t.Fatal("owned evidence directory required")
	}
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	kube, err := NewE2EKube("docker-desktop", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	env := &E2EEnvironment{Kube: kube, Options: E2EOptions{KubeContext: "docker-desktop", Artifacts: artifacts, RunID: "plan7-storage-" + uuid.NewString()}}
	if run := os.Getenv("ARGUS_P7_STORAGE_RUN_ID"); run != "" {
		env.Options.RunID = run
	}
	if err = (&App{}).plan7WorkspaceDevicePrerequisites(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	name := "plan7-storage-" + uuid.NewString()[:8]
	ns, err := kube.Client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/release-id": env.Options.RunID, "pod-security.kubernetes.io/enforce": "restricted"}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("owned storage namespace=%s uid=%s", name, ns.UID)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := kube.DeleteNamespace(ctx, name, env.Options.RunID, string(ns.UID)); err != nil {
			t.Error(err)
		}
	})
	class := "argus-workspace"
	volumes := []corev1.Volume{}
	mounts := []corev1.VolumeMount{}
	claims := []string{}
	for n := 0; n < 9; n++ {
		claim := fmt.Sprintf("evidence-%d", n)
		pvc, err := kube.Client.CoreV1().PersistentVolumeClaims(name).Create(t.Context(), &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: claim, Labels: map[string]string{"argus.io/release-id": env.Options.RunID}}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, StorageClassName: &class, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("16Mi")}}}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		claims = append(claims, string(pvc.UID))
		volumes = append(volumes, corev1.Volume{Name: claim, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}})
		mounts = append(mounts, corev1.VolumeMount{Name: claim, MountPath: fmt.Sprintf("/evidence/%d", n)})
	}
	image := os.Getenv("ARGUS_P7_STORAGE_IMAGE")
	if image == "" {
		t.Fatal("pinned cached test image required")
	}
	no, yes := false, true
	uid := int64(65532)
	marker := uuid.NewString()
	for _, phase := range []string{"write", "read"} {
		script := `set -eu; for dir in /evidence/*; do `
		if phase == "write" {
			script += `printf '%s' "$P7_MARKER" > "$dir/proof"; `
		}
		script += `test "$(cat "$dir/proof")" = "$P7_MARKER"; done; printf 'nine volumes verified\n'`
		pod, err := kube.Client.CoreV1().Pods(name).Create(t.Context(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: phase, Labels: map[string]string{"argus.io/release-id": env.Options.RunID}}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: &no, SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &yes, RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Volumes: volumes, Containers: []corev1.Container{{Name: "probe", Image: image, ImagePullPolicy: corev1.PullNever, Command: []string{"/bin/bash", "-ec", script}, Env: []corev1.EnvVar{{Name: "P7_MARKER", Value: marker}}, VolumeMounts: mounts, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}}}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for {
			current, e := kube.Client.CoreV1().Pods(name).Get(t.Context(), phase, metav1.GetOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if current.Status.Phase == corev1.PodSucceeded {
				break
			}
			if current.Status.Phase == corev1.PodFailed || time.Now().After(deadline) {
				t.Fatalf("nine-volume %s did not complete: %s", phase, current.Status.Phase)
			}
			time.Sleep(time.Second)
		}
		if err = kube.Client.CoreV1().Pods(name).Delete(t.Context(), phase, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pod.UID}}); err != nil {
			t.Fatal(err)
		}
		for {
			_, e := kube.Client.CoreV1().Pods(name).Get(t.Context(), phase, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				break
			}
			if e != nil || time.Now().After(deadline) {
				t.Fatalf("probe Pod not removed: %v", e)
			}
			time.Sleep(time.Second)
		}
	}
	raw, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "namespace": name, "namespace_uid": ns.UID, "claim_uids": claims, "simultaneous_volumes": 9, "writer_and_fresh_reader_passed": true}, "", "  ")
	if err = os.WriteFile(filepath.Join(artifacts, "plan7-storage-nine-volumes.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
