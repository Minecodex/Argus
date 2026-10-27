// Package kubernetesstream owns native, UID-bound Kubernetes exec and TCP
// forwarding. It has no control-plane authorization or browser ticket store.
package kubernetesstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/kakj-go/Argus/internal/kubernetesaccess"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const SchemaVersion = "argus.kubernetes_stream/v1"
const WebSocketProtocol = "argus.kubernetes_stream/v1"

var ErrProtocol = errors.New("KUBERNETES_STREAM_PROTOCOL_INVALID")

type ServicePin struct {
	Name     string `json:"name"`
	UID      string `json:"uid"`
	Port     int32  `json:"port"`
	PortName string `json:"port_name"`
}
type Target struct {
	SchemaVersion       string                             `json:"schema_version"`
	APIServer           string                             `json:"api_server"`
	Mode                string                             `json:"mode"`
	Pin                 kubernetespodlogs.Pin              `json:"pin"`
	Shell               string                             `json:"shell,omitempty"`
	Port                int32                              `json:"port,omitempty"`
	Service             *ServicePin                        `json:"service,omitempty"`
	SecurityFingerprint string                             `json:"security_fingerprint"`
	ClusterWide         bool                               `json:"cluster_wide"`
	Policy              kubernetesinventory.MutationPolicy `json:"policy"`
}

func (t Target) Validate() error {
	p := t.Pin
	if t.SchemaVersion != SchemaVersion || !kubernetesaccess.ValidServer(t.APIServer) || p.Namespace == "" || p.NamespaceUID == "" || p.Pod == "" || p.PodUID == "" || p.Container == "" || p.ContainerID == "" || p.RestartCount < 0 || p.Previous || len(t.SecurityFingerprint) != 64 {
		return ErrProtocol
	}
	switch t.Mode {
	case "exec":
		if !slices.Contains([]string{"sh", "bash", "powershell", "cmd"}, t.Shell) || t.Port != 0 || t.Service != nil {
			return ErrProtocol
		}
	case "port_forward":
		if t.Shell != "" || t.Port < 1 || t.Port > 65535 {
			return ErrProtocol
		}
	default:
		return ErrProtocol
	}
	if t.Service != nil && (t.Service.Name == "" || t.Service.UID == "" || t.Service.Port < 1 || t.Service.Port > 65535) {
		return ErrProtocol
	}
	return nil
}
func SecurityFingerprint(pod *corev1.Pod) string {
	type container struct {
		Name     string
		Security *corev1.SecurityContext
		Mounts   []corev1.VolumeMount
	}
	containers := []container{}
	for _, group := range [][]corev1.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
		for _, c := range group {
			containers = append(containers, container{c.Name, c.SecurityContext, c.VolumeMounts})
		}
	}
	for _, c := range pod.Spec.EphemeralContainers {
		containers = append(containers, container{c.Name, c.SecurityContext, c.VolumeMounts})
	}
	raw, _ := json.Marshal([]any{pod.Spec.HostNetwork, pod.Spec.HostPID, pod.Spec.HostIPC, pod.Spec.NodeName, pod.Spec.ServiceAccountName, pod.Spec.AutomountServiceAccountToken, pod.Spec.SecurityContext, pod.Spec.Volumes, containers})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
func ValidatePodPolicy(pod *corev1.Pod, clusterWide bool, policy kubernetesinventory.MutationPolicy) error {
	// Typed client-go objects do not retain TypeMeta after decoding.
	canonical := pod.DeepCopy()
	canonical.APIVersion, canonical.Kind = "v1", "Pod"
	raw, _ := json.Marshal(canonical)
	definition, _ := kubernetesinventory.Lookup("pod")
	return kubernetesinventory.ValidateMutationResult(definition, kubernetesinventory.MutationResult{After: &kubernetesinventory.Object{Data: raw}}, clusterWide, policy)
}
func ValidateTarget(ctx context.Context, api kubernetes.Interface, target Target) error {
	if err := target.Validate(); err != nil {
		return err
	}
	namespace, err := api.CoreV1().Namespaces().Get(ctx, target.Pin.Namespace, metav1.GetOptions{})
	if err != nil {
		return accessError(err)
	}
	if string(namespace.UID) != target.Pin.NamespaceUID || namespace.DeletionTimestamp != nil {
		return kubernetesinventory.ErrIdentityChanged
	}
	pod, err := api.CoreV1().Pods(target.Pin.Namespace).Get(ctx, target.Pin.Pod, metav1.GetOptions{})
	if err != nil {
		return accessError(err)
	}
	pin, err := kubernetespodlogs.PinPod(target.Pin.NamespaceUID, pod, target.Pin.Container, false)
	pin.PreviousContainerID = ""
	if err != nil || pin != target.Pin || SecurityFingerprint(pod) != target.SecurityFingerprint {
		return kubernetesinventory.ErrIdentityChanged
	}
	if err = ValidatePodPolicy(pod, target.ClusterWide, target.Policy); err != nil {
		return err
	}
	if target.Service != nil {
		return validateService(ctx, api, target)
	}
	return nil
}

func accessError(err error) error {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return kubernetesinventory.ErrForbidden
	}
	if apierrors.IsNotFound(err) {
		return kubernetesinventory.ErrIdentityChanged
	}
	return kubernetesinventory.ErrUnavailable
}

func ShellCommand(shell string) []string {
	switch shell {
	case "sh":
		return []string{"/bin/sh"}
	case "bash":
		return []string{"/bin/bash", "--noprofile", "--norc"}
	case "powershell":
		return []string{"powershell.exe", "-NoLogo", "-NoProfile"}
	case "cmd":
		return []string{"cmd.exe", "/D"}
	}
	return nil
}
