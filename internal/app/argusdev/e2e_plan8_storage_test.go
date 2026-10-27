package argusdev

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPlan8DevicePrerequisitesRequireEveryOwnedReadyCSINode(t *testing.T) {
	for _, failure := range []string{"", "foreign_namespace", "foreign_driver", "foreign_controller", "wrong_image", "wrong_mount", "wrong_node", "missing_node", "unready", "wrong_suite", "foreign_context"} {
		t.Run(failure, func(t *testing.T) {
			const ns, release = "owned-system", "owned-release"
			cluster := "plan8-owned-test"
			env := &E2EEnvironment{SystemNS: ns, ReleaseID: release, Options: E2EOptions{Suite: "plan8", KubeContext: "kind-" + cluster}}
			annotations := map[string]string{"meta.helm.sh/release-namespace": ns, "meta.helm.sh/release-name": "owned-workspace"}
			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns, UID: types.UID("namespace-generation"), Labels: map[string]string{"argus.io/release-id": release}}}
			driver := &storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: "rawfile.csi.openebs.io", Annotations: annotations}}
			set := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "rawfile-node", Namespace: ns, UID: types.UID("daemonset-generation"), Annotations: annotations}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "rawfile-node"}}}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 2}}
			controller := true
			pods := []*corev1.Pod{}
			for _, suffix := range []string{"control-plane", "worker"} {
				pods = append(pods, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "rawfile-" + suffix, Namespace: ns, UID: types.UID("pod-" + suffix), Labels: map[string]string{"app": "rawfile-node"}, OwnerReferences: []metav1.OwnerReference{{UID: set.UID, Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: cluster + "-" + suffix, Volumes: []corev1.Volume{{Name: "devices", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/dev"}}}}, Containers: []corev1.Container{{Name: "csi-driver", VolumeMounts: []corev1.VolumeMount{{Name: "devices", MountPath: "/dev"}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "csi-driver", Ready: true, ImageID: "docker.io/openebs/rawfile-localpv@" + plan8RawfileAMD64Digest}}}})
			}
			switch failure {
			case "foreign_namespace":
				namespace.Labels["argus.io/release-id"] = "another-release"
			case "foreign_driver":
				driver.Annotations = map[string]string{"meta.helm.sh/release-namespace": "unrelated-system", "meta.helm.sh/release-name": "unrelated-workspace"}
			case "foreign_controller":
				pods[1].OwnerReferences[0].UID = "another-controller"
			case "wrong_image":
				pods[1].Status.ContainerStatuses[0].ImageID = "docker.io/unverified@sha256:" + strings.Repeat("a", 64)
			case "wrong_mount":
				pods[1].Spec.Volumes[0].HostPath.Path = "/unrelated-devices"
			case "wrong_node":
				pods[1].Spec.NodeName = "customer-production-worker"
			case "missing_node":
				pods = pods[:1]
			case "unready":
				pods[1].Status.ContainerStatuses[0].Ready = false
			case "wrong_suite":
				env.Options.Suite = "plan7"
			case "foreign_context":
				env.Options.KubeContext = "kind-customer-production"
			}
			objects := []runtime.Object{namespace, driver, set}
			for _, pod := range pods {
				objects = append(objects, pod)
			}
			client := fake.NewClientset(objects...)
			env.Kube = &E2EKube{Client: client}
			result, err := plan8OwnedWorkspaceDevicePods(t.Context(), env, func(_, actual string) (bool, error) {
				return strings.HasSuffix(actual, "@"+plan8RawfileAMD64Digest), nil
			})
			if failure == "" {
				if err != nil || len(result) != 2 {
					t.Fatalf("two verified node Pods were not admitted: %v", err)
				}
			} else if err == nil {
				t.Fatal("unowned, incomplete or unverified CSI admitted")
			}
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" && action.GetVerb() != "list" {
					t.Fatal("device verification performed a cluster mutation")
				}
			}
		})
	}
}

func TestPlan8DeviceVerifierHasNoRepairOperations(t *testing.T) {
	for _, forbidden := range []string{"mknod", "makedev", "chmod", "losetup", "open(", "unlink", "mkdir", "mount("} {
		if strings.Contains(plan8LoopDeviceReadOnlyScript, forbidden) {
			t.Fatal("read-only acceptance contains a device repair or attachment operation", forbidden)
		}
	}
}
