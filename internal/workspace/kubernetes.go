package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const workspaceLabel = "argus.io/workspace-id"
const fenceLabel = "argus.io/workspace-fence"

func intPort(port int) intstr.IntOrString { return intstr.FromInt(port) }

func ownedFence(labels map[string]string, maximum int64) (bool, error) {
	fence, err := strconv.ParseInt(labels[fenceLabel], 10, 64)
	if err != nil || fence <= 0 {
		return false, toolruntime.Error{Kind: "WORKSPACE_OWNERSHIP_INVALID"}
	}
	return fence <= maximum, nil
}

type Kubernetes struct {
	Client kubernetes.Interface
	Config config.Workspace
}

func (runtime Kubernetes) Ready(ctx context.Context) error {
	if runtime.Client == nil || runtime.Config.IOImage == "" {
		return toolruntime.Error{Kind: "WORKSPACE_NOT_CONFIGURED"}
	}
	class, err := runtime.Client.StorageV1().StorageClasses().Get(ctx, runtime.Config.StorageClass, metav1.GetOptions{})
	if err != nil {
		return toolruntime.Error{Kind: "WORKSPACE_STORAGE_UNAVAILABLE"}
	}
	if class.Provisioner != "rawfile.csi.openebs.io" || class.Parameters["thinProvision"] != "false" || class.Parameters["csi.storage.k8s.io/fstype"] != "ext4" || class.VolumeBindingMode == nil || string(*class.VolumeBindingMode) != "WaitForFirstConsumer" || class.AllowVolumeExpansion != nil && *class.AllowVolumeExpansion || !slices.Contains(class.MountOptions, "nodiscard") {
		return toolruntime.Error{Kind: "WORKSPACE_HARD_QUOTA_UNAVAILABLE"}
	}
	return nil
}

func (runtime Kubernetes) EnsureVolume(ctx context.Context, workspace db.Workspace) error {
	if err := runtime.Ready(ctx); err != nil {
		return err
	}
	class := runtime.Config.StorageClass
	_, err := runtime.Client.CoreV1().PersistentVolumeClaims(workspace.Namespace).Create(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: workspace.PvcName, Namespace: workspace.Namespace, Labels: map[string]string{workspaceLabel: workspace.ID.String(), "argus.io/enterprise-id": workspace.EnterpriseID.String()}}, Spec: corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, StorageClassName: &class, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: *resource.NewQuantity(workspace.CapacityBytes, resource.BinarySI)}}}}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		claim, err := runtime.Client.CoreV1().PersistentVolumeClaims(workspace.Namespace).Get(ctx, workspace.PvcName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if claim.Labels[workspaceLabel] != workspace.ID.String() || claim.Labels["argus.io/enterprise-id"] != workspace.EnterpriseID.String() {
			return toolruntime.Error{Kind: "WORKSPACE_STORAGE_FORBIDDEN"}
		}
		return nil
	}
	return err
}

func (runtime Kubernetes) CreateIO(ctx context.Context, workspace db.Workspace) (*corev1.Pod, error) {
	uid := int64(workspace.RuntimeUid)
	no := false
	yes := true
	mode := int32(0440)
	name := fmt.Sprintf("workspace-io-%s-%d", workspace.ID.String()[:16], workspace.FenceToken)
	return runtime.Client.CoreV1().Pods(workspace.Namespace).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: workspace.Namespace,
		Labels: map[string]string{workspaceLabel: workspace.ID.String(), fenceLabel: fmt.Sprint(workspace.FenceToken), "argus.io/workspace-mode": "io"}}, Spec: corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: &no,
		SecurityContext: &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid, RunAsNonRoot: &yes},
		Containers: []corev1.Container{{Name: "workspace-io", Image: runtime.Config.IOImage, Command: []string{"/usr/local/bin/argus-worker", "--pool=workspace-io"},
			Env: runtime.IOEnv(workspace), Ports: []corev1.ContainerPort{{Name: "file-rpc", ContainerPort: 8447}},
			SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
			Resources:       runtime.IOResources(),
			VolumeMounts:    []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}, {Name: "workspace-tls", MountPath: "/var/run/argus/workspace", ReadOnly: true}},
			ReadinessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intPort(8447)}}, PeriodSeconds: 1, FailureThreshold: 60}}},
		Volumes: []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: workspace.PvcName}}},
			{Name: "workspace-tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: runtime.Config.TLSSecret, DefaultMode: &mode}}}}}}, metav1.CreateOptions{})
}

func (runtime Kubernetes) IOEnv(workspace db.Workspace) []corev1.EnvVar {
	return []corev1.EnvVar{{Name: "ARGUS_WORKSPACE_ID", Value: workspace.ID.String()}, {Name: "ARGUS_WORKSPACE_FENCE", Value: fmt.Sprint(workspace.FenceToken)},
		{Name: "ARGUS_WORKSPACE_TLS_CERT", Value: "/var/run/argus/workspace/tls.crt"}, {Name: "ARGUS_WORKSPACE_TLS_KEY", Value: "/var/run/argus/workspace/tls.key"},
		{Name: "ARGUS_WORKSPACE_TLS_CA", Value: "/var/run/argus/workspace/ca.crt"}, {Name: "ARGUS_WORKSPACE_CLIENT_NAME", Value: runtime.Config.ClientName},
		{Name: "ARGUS_WORKSPACE_MAX_FILE_BYTES", Value: fmt.Sprint(runtime.Config.MaxFileBytes)},
		{Name: "ARGUS_WORKSPACE_IDLE_SECONDS", Value: fmt.Sprint(max(1, int(runtime.Config.IdleTTL.Seconds())))}}
}

func (runtime Kubernetes) WaitPod(ctx context.Context, workspace db.Workspace) (*corev1.Pod, error) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		pods, err := runtime.Client.CoreV1().Pods(workspace.Namespace).List(ctx, metav1.ListOptions{LabelSelector: workspaceLabel + "=" + workspace.ID.String() + "," + fenceLabel + "=" + fmt.Sprint(workspace.FenceToken)})
		if err != nil {
			return nil, err
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodFailed {
				return nil, toolruntime.Error{Kind: "WORKSPACE_RUNTIME_UNAVAILABLE"}
			}
			if pod.DeletionTimestamp != nil || pod.Status.PodIP == "" {
				continue
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					return &pod, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Retire revokes the creating workload as well as Pods. Otherwise a controller
// could recreate an old writer after a new lease has attached the volume.
func (runtime Kubernetes) Retire(ctx context.Context, workspace db.Workspace) error {
	selector := workspaceLabel + "=" + workspace.ID.String()
	if rest := runtime.Client.CoreV1().RESTClient(); rest != nil {
		path := "/apis/sandbox.opensandbox.io/v1alpha1/namespaces/" + workspace.Namespace + "/batchsandboxes"
		raw, err := rest.Get().AbsPath(path).Param("labelSelector", selector).Do(ctx).Raw()
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err == nil {
			var list struct {
				Items []struct {
					Metadata metav1.ObjectMeta `json:"metadata"`
				} `json:"items"`
			}
			if json.Unmarshal(raw, &list) != nil {
				return toolruntime.Error{Kind: "WORKSPACE_RUNTIME_UNAVAILABLE"}
			}
			for _, item := range list.Items {
				owned, err := ownedFence(item.Metadata.Labels, workspace.FenceToken)
				if err != nil {
					return err
				}
				if !owned {
					continue
				}
				policy := metav1.DeletePropagationForeground
				if err := rest.Delete().AbsPath(path + "/" + item.Metadata.Name).Body(&metav1.DeleteOptions{PropagationPolicy: &policy}).Do(ctx).Error(); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}
	pods, err := runtime.Client.CoreV1().Pods(workspace.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		owned, err := ownedFence(pod.Labels, workspace.FenceToken)
		if err != nil {
			return err
		}
		if !owned {
			continue
		}
		grace := int64(1)
		if err := runtime.Client.CoreV1().Pods(workspace.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace, Preconditions: &metav1.Preconditions{UID: &pod.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		pods, err := runtime.Client.CoreV1().Pods(workspace.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return err
		}
		pending := false
		for _, pod := range pods.Items {
			owned, err := ownedFence(pod.Labels, workspace.FenceToken)
			if err != nil {
				return err
			}
			pending = pending || owned
		}
		if !pending {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// The literal editor holds at most one file buffer. Leave bounded runtime and
// RPC headroom above the platform-controlled file limit.
func (runtime Kubernetes) IOResources() corev1.ResourceRequirements {
	memory := runtime.Config.MaxFileBytes + (128 << 20)
	if memory < 256<<20 {
		memory = 256 << 20
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI)},
	}
}
