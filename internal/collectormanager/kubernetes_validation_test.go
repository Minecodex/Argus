package collectormanager

import (
	"slices"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubernetesCandidateMountsActualAgentKubeletTrust(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		job.Status.Succeeded = 1
		return false, nil, nil
	})
	command := collectorCommand(t, testConfigBundle(t), "linux_amd64", "https://artifacts.example/collector.tar.gz", []byte("artifact"))
	command.ResourceType = "kubernetes_cluster"
	command.KubernetesImage = "registry.example/collector:test"
	if err := validateKubernetesCandidate(t.Context(), client, command, command.KubernetesImage, configSlotNext); err != nil {
		t.Fatal(err)
	}
	job, err := client.BatchV1().Jobs(KubernetesNamespace).Get(t.Context(), collectorValidationJob, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	agent := job.Spec.Template.Spec.InitContainers[0]
	if !slices.ContainsFunc(agent.VolumeMounts, func(mount corev1.VolumeMount) bool {
		return mount.Name == "kubelet-pki" && mount.MountPath == "/var/run/argus-kubelet/pki" && mount.ReadOnly
	}) {
		t.Fatal("candidate cannot read the kubelet CA used by the real agent")
	}
	if !slices.ContainsFunc(job.Spec.Template.Spec.Volumes, func(volume corev1.Volume) bool {
		return volume.Name == "kubelet-pki" && volume.HostPath != nil && volume.HostPath.Path == "/var/lib/kubelet/pki"
	}) {
		t.Fatal("candidate kubelet trust volume missing")
	}
	if len(agent.Args) == 0 || agent.Args[0] != "validate" {
		t.Fatal("candidate would start collection")
	}
	for _, volume := range job.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			t.Fatal("candidate mounts the active single-writer PVC")
		}
	}
}
