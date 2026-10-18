package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Docker Desktop's node /dev can expose only loop0..7. RawFile v0.15.1 cannot
// parse util-linux's '/dev/loopN (lost)' result when the next node is missing.
// Provision standard device aliases, without attaching, formatting, detaching,
// deleting, or touching any backing file. Existing aliases must match exactly.
const plan7LoopDeviceScript = `import json, os, stat
created = []
if os.geteuid() != 0:
    raise RuntimeError('loop device prerequisites require the existing CSI identity')
for index in range(64):
    path = '/dev/loop' + str(index)
    try:
        info = os.lstat(path)
    except FileNotFoundError:
        try:
            os.mknod(path, stat.S_IFBLK | 0o600, os.makedev(7, index))
            created.append(path)
        except FileExistsError:
            pass
        info = os.lstat(path)
    if not stat.S_ISBLK(info.st_mode) or os.major(info.st_rdev) != 7 or os.minor(info.st_rdev) != index:
        raise RuntimeError('unexpected existing device: ' + path)
print(json.dumps({'verified_nodes': 64, 'created': created, 'attachments_changed': False, 'backing_files_changed': False}))
`

func (a *App) plan7WorkspaceDevicePrerequisites(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.KubeContext != "docker-desktop" {
		return fmt.Errorf("Plan7 node prerequisites are restricted to the approved docker-desktop test environment")
	}
	driver, err := env.Kube.Client.StorageV1().CSIDrivers().Get(ctx, "rawfile.csi.openebs.io", metav1.GetOptions{})
	if err != nil {
		return err
	}
	namespace, release := driver.Annotations["meta.helm.sh/release-namespace"], driver.Annotations["meta.helm.sh/release-name"]
	if namespace == "" || release == "" {
		return fmt.Errorf("RawFile driver ownership missing")
	}
	sets, err := env.Kube.Client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	const digest = "sha256:eb99a5caf5b2a87471b0fba4d67e310aae4fe06b663eb9c85bfa96f49a545446"
	for _, set := range sets.Items {
		if set.Annotations["meta.helm.sh/release-name"] != release || set.Annotations["meta.helm.sh/release-namespace"] != namespace {
			continue
		}
		selector := labels.SelectorFromSet(set.Spec.Selector.MatchLabels).String()
		pods, e := env.Kube.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if e != nil {
			return e
		}
		if len(pods.Items) != 1 {
			return fmt.Errorf("Plan7 loop prerequisites require one stable local CSI Pod")
		}
		pod := pods.Items[0]
		if pod.Spec.NodeName != "desktop-control-plane" {
			return fmt.Errorf("Plan7 loop prerequisite repair is restricted to the verified local desktop-control-plane node")
		}
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.UID != set.UID || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			return fmt.Errorf("CSI Pod identity changed")
		}
		verified := false
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == "csi-driver" && status.Ready && strings.HasSuffix(status.ImageID, "@"+digest) {
				verified = true
			}
		}
		if !verified {
			return fmt.Errorf("CSI runtime does not match the approved RawFile digest")
		}
		deviceVolume := ""
		for _, volume := range pod.Spec.Volumes {
			if volume.HostPath != nil && volume.HostPath.Path == "/dev" {
				deviceVolume = volume.Name
			}
		}
		mounted := false
		for _, container := range pod.Spec.Containers {
			if container.Name == "csi-driver" {
				for _, mount := range container.VolumeMounts {
					if deviceVolume != "" && mount.Name == deviceVolume && mount.MountPath == "/dev" && !mount.ReadOnly {
						mounted = true
					}
				}
			}
		}
		if !mounted {
			return fmt.Errorf("CSI device mount identity differs")
		}
		output, e := env.Kube.Exec(ctx, namespace, selector, "csi-driver", "python3", "-c", plan7LoopDeviceScript)
		if e != nil {
			return e
		}
		var result map[string]any
		if json.Unmarshal([]byte(output), &result) != nil || result["verified_nodes"] != float64(64) {
			return fmt.Errorf("loop prerequisite verification receipt invalid")
		}
		raw, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "node": pod.Spec.NodeName, "daemonset_uid": set.UID, "pod_uid": pod.UID, "image_digest": digest, "result": result, "scope": "standard Docker Desktop loop device aliases; existing devices and all data preserved"}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-loop-device-prerequisites.json"), raw)
	}
	return fmt.Errorf("owned RawFile CSI workload not found")
}
