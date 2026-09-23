package workspace

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAdmissionHardensFileRoleAndRejectsUntrustedSources(t *testing.T) {
	cfg := config.Workspace{IOImage: "argus-worker@sha256:test", TLSSecret: "workspace-file-tls", MaxFileBytes: 100 << 20}
	runtime := Kubernetes{Client: fake.NewSimpleClientset(), Config: cfg}
	workspace := db.Workspace{ID: uuid.New(), EnterpriseID: uuid.New(), PvcName: "owned-volume", Namespace: "workspace-tests", RuntimeUid: 20000, FenceToken: 1}
	original, err := runtime.CreateIO(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Config: cfg, Kubernetes: runtime}
	request := httptest.NewRequest("POST", "/admission", nil)
	t.Run("service account removed and non-root enforced", func(t *testing.T) {
		pod := original.DeepCopy()
		root, yes := int64(0), true
		pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: &yes, RunAsUser: &root}
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: "kube-token", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token"}}}}}})
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "kube-token", MountPath: "/token"})
		if err := service.hardenPod(request, pod, workspace); err != nil {
			t.Fatal(err)
		}
		security := pod.Spec.Containers[0].SecurityContext
		if security.Privileged != nil && *security.Privileged || *security.RunAsUser != 20000 || !*security.RunAsNonRoot || !*security.ReadOnlyRootFilesystem || *security.AllowPrivilegeEscalation {
			t.Fatal("file role was not hardened")
		}
		if *pod.Spec.AutomountServiceAccountToken || *pod.Spec.ShareProcessNamespace || len(pod.Spec.Volumes) != 2 || len(pod.Spec.Containers[0].VolumeMounts) != 2 {
			t.Fatal("service account or process namespace remained exposed")
		}
	})
	attacks := map[string]func(*corev1.Pod){
		"host network":    func(p *corev1.Pod) { p.Spec.HostNetwork = true },
		"other workspace": func(p *corev1.Pod) { p.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "another-workspace" },
		"host filesystem": func(p *corev1.Pod) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}})
		},
		"remote filesystem": func(p *corev1.Pod) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "remote", VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: "server", Path: "/"}}})
		},
		"other credentials": func(p *corev1.Pod) { p.Spec.Volumes[1].Secret.SecretName = "database-credentials" },
		"credential env": func(p *corev1.Pod) {
			p.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "database"}}}}
		},
		"credential key": func(p *corev1.Pod) {
			p.Spec.Containers[0].Env = append(p.Spec.Containers[0].Env, corev1.EnvVar{Name: "KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "database"}, Key: "password"}}})
		},
		"arbitrary file role command": func(p *corev1.Pod) { p.Spec.Containers[0].Command = []string{"bash", "-c", "id"} },
		"unapproved image":            func(p *corev1.Pod) { p.Spec.Containers[0].Image = "customer/image" },
	}
	for name, mutate := range attacks {
		t.Run(name, func(t *testing.T) {
			pod := original.DeepCopy()
			mutate(pod)
			if err := service.hardenPod(request, pod, workspace); err == nil {
				t.Fatal("unsafe Pod accepted")
			}
		})
	}
}
