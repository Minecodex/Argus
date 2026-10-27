package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const plan8RawfileIndexDigest = "sha256:eb99a5caf5b2a87471b0fba4d67e310aae4fe06b663eb9c85bfa96f49a545446"
const plan8RawfileAMD64Digest = "sha256:9f8042417d8edfb37ce1e2a6dba51d8a246ccae5e2fcda9f35d1fc393887e56b"

// Kind verification never repairs devices. Environment preparation is separate
// from acceptance; Docker Desktop retains its existing explicitly scoped path.
const plan8LoopDeviceReadOnlyScript = `import json, os, stat
for index in range(64):
    info = os.lstat('/dev/loop' + str(index))
    if not stat.S_ISBLK(info.st_mode) or os.major(info.st_rdev) != 7 or os.minor(info.st_rdev) != index:
        raise RuntimeError('loop device prerequisite identity mismatch')
info = os.lstat('/dev/loop-control')
if not stat.S_ISCHR(info.st_mode) or os.major(info.st_rdev) != 10 or os.minor(info.st_rdev) != 237:
    raise RuntimeError('loop control prerequisite identity mismatch')
print(json.dumps({'verified_device_aliases':64,'loop_control_verified':True,'mutations':0}))
`

func (a *App) plan8WorkspaceDevicePrerequisites(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.KubeContext == "docker-desktop" {
		return a.plan7WorkspaceDevicePrerequisites(ctx, env)
	}
	pods, err := plan8OwnedWorkspaceDevicePods(ctx, env, func(node, actual string) (bool, error) {
		return a.plan8RawfileNodeImageIdentity(ctx, env, node, actual)
	})
	if err != nil {
		return err
	}
	receipts := []map[string]any{}
	for _, pod := range pods {
		output, e := env.Kube.execPod(ctx, pod.Namespace, pod.Name, "csi-driver", "python3", "-c", plan8LoopDeviceReadOnlyScript)
		if e != nil {
			return fmt.Errorf("owned CSI node %s read-only device prerequisites: %w", pod.Spec.NodeName, e)
		}
		var result struct {
			Verified  int  `json:"verified_device_aliases"`
			Control   bool `json:"loop_control_verified"`
			Mutations *int `json:"mutations"`
		}
		if json.Unmarshal([]byte(output), &result) != nil || result.Verified != 64 || !result.Control || result.Mutations == nil || *result.Mutations != 0 {
			return errors.New("owned CSI read-only prerequisite receipt invalid")
		}
		current, e := env.Kube.Client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if e != nil || current.UID != pod.UID || current.DeletionTimestamp != nil || current.Spec.NodeName != pod.Spec.NodeName {
			return errors.New("owned CSI Pod changed during read-only verification")
		}
		imageID := ""
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == "csi-driver" {
				imageID = status.ImageID
			}
		}
		receipts = append(receipts, map[string]any{"node": pod.Spec.NodeName, "pod_uid": pod.UID, "daemonset_uid": metav1.GetControllerOf(&pod).UID, "image_id": imageID, "verified_device_aliases": result.Verified, "loop_control_verified": result.Control, "mutations": *result.Mutations})
	}
	raw, err := json.MarshalIndent(map[string]any{"schema_version": "argus.plan8_workspace_device_prerequisites/v1", "passed": true, "run_id": env.Options.RunID, "scope": "owned_plan8_kind_csi_read_only_device_verification", "nodes": receipts, "mutations": 0}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-loop-device-prerequisites.json"), raw)
}

func plan8OwnedWorkspaceDevicePods(ctx context.Context, env *E2EEnvironment, verifyImage func(string, string) (bool, error)) ([]corev1.Pod, error) {
	if env.Options.Suite != "plan8" || !strings.HasPrefix(env.Options.KubeContext, "kind-plan8-") || env.SystemNS == "" || env.ReleaseID == "" || verifyImage == nil {
		return nil, errors.New("read-only device verification requires the owned Plan8 Kind environment")
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.UID == "" || ns.DeletionTimestamp != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return nil, errors.New("owned workspace namespace identity changed")
	}
	driver, err := env.Kube.Client.StorageV1().CSIDrivers().Get(ctx, "rawfile.csi.openebs.io", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	release := driver.Annotations["meta.helm.sh/release-name"]
	if driver.Annotations["meta.helm.sh/release-namespace"] != env.SystemNS || release == "" {
		return nil, errors.New("workspace CSI is not owned by this installation")
	}
	sets, err := env.Kube.Client.AppsV1().DaemonSets(env.SystemNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := []corev1.Pod{}
	seen := map[string]bool{}
	for _, set := range sets.Items {
		if set.Annotations["meta.helm.sh/release-name"] != release || set.Annotations["meta.helm.sh/release-namespace"] != env.SystemNS {
			continue
		}
		if set.UID == "" || set.Spec.Selector == nil || len(set.Spec.Selector.MatchLabels) == 0 || set.Status.DesiredNumberScheduled == 0 {
			return nil, errors.New("owned CSI DaemonSet identity or scheduling unavailable")
		}
		pods, e := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(set.Spec.Selector.MatchLabels).String()})
		if e != nil || len(pods.Items) != int(set.Status.DesiredNumberScheduled) {
			return nil, errors.New("owned CSI requires every scheduled node Pod")
		}
		for _, pod := range pods.Items {
			owner := metav1.GetControllerOf(&pod)
			cluster := strings.TrimPrefix(env.Options.KubeContext, "kind-")
			if pod.UID == "" || owner == nil || owner.UID != set.UID || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || !strings.HasPrefix(pod.Spec.NodeName, cluster+"-") || seen[pod.Spec.NodeName] {
				return nil, errors.New("owned CSI Pod or node identity changed")
			}
			ready, mounted := false, false
			deviceVolume := ""
			for _, volume := range pod.Spec.Volumes {
				if volume.HostPath != nil && volume.HostPath.Path == "/dev" {
					deviceVolume = volume.Name
				}
			}
			for _, container := range pod.Spec.Containers {
				if container.Name == "csi-driver" {
					for _, mount := range container.VolumeMounts {
						mounted = mounted || deviceVolume != "" && mount.Name == deviceVolume && mount.MountPath == "/dev"
					}
				}
			}
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == "csi-driver" && status.Ready {
					var imageErr error
					ready, imageErr = verifyImage(pod.Spec.NodeName, status.ImageID)
					if imageErr != nil {
						return nil, fmt.Errorf("owned CSI image descriptor verification: %w", imageErr)
					}
				}
			}
			if !ready || !mounted {
				return nil, errors.New("owned CSI runtime digest or node device mount differs")
			}
			seen[pod.Spec.NodeName] = true
			result = append(result, pod)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("owned RawFile CSI Pods not found")
	}
	return result, nil
}
