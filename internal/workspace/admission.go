package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (service Service) Admission(w http.ResponseWriter, r *http.Request) {
	var review admissionv1.AdmissionReview
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&review) != nil || review.Request == nil {
		http.Error(w, "invalid admission review", http.StatusBadRequest)
		return
	}
	response := &admissionv1.AdmissionResponse{UID: review.Request.UID, Allowed: false}
	deny := func(message string) {
		response.Result = &metav1.Status{Message: message, Reason: metav1.StatusReasonForbidden}
	}
	var pod corev1.Pod
	if json.Unmarshal(review.Request.Object.Raw, &pod) != nil {
		deny("invalid workspace Pod")
	} else if review.Request.Operation != admissionv1.Create || review.Request.Namespace != service.Config.Namespace {
		deny("workspace admission scope mismatch")
	} else {
		id, err := uuid.Parse(pod.Labels[workspaceLabel])
		fence, parseErr := strconv.ParseInt(pod.Labels[fenceLabel], 10, 64)
		if err != nil || parseErr != nil || fence <= 0 {
			deny("workspace identity missing")
		} else {
			workspace, err := service.Store.Queries.GetWorkspaceForAdmission(r.Context(), db.GetWorkspaceForAdmissionParams{ID: id, Namespace: review.Request.Namespace, FenceToken: fence})
			if err != nil {
				deny("workspace lease is not current")
			} else if err := service.hardenPod(r, &pod, workspace); err != nil {
				deny(err.Error())
			} else {
				patch, _ := json.Marshal([]map[string]any{{"op": "replace", "path": "/spec", "value": pod.Spec}})
				kind := admissionv1.PatchTypeJSONPatch
				response.Allowed = true
				response.PatchType = &kind
				response.Patch = patch
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(admissionv1.AdmissionReview{TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"}, Response: response})
}

type admissionError string

func (err admissionError) Error() string { return string(err) }

func (service Service) hardenPod(r *http.Request, pod *corev1.Pod, workspace db.Workspace) error {
	if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
		return admissionError("workspace host namespaces are forbidden")
	}
	if err := service.validatePodSources(pod, workspace); err != nil {
		return err
	}
	volumeFound := false
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil {
			return admissionError("workspace host mounts are forbidden")
		}
		if volume.PersistentVolumeClaim != nil {
			if volume.PersistentVolumeClaim.ClaimName != workspace.PvcName {
				return admissionError("workspace volume ownership mismatch")
			}
			volumeFound = true
		}
	}
	if !volumeFound {
		return admissionError("workspace persistent volume missing")
	}
	uid := int64(workspace.RuntimeUid)
	yes, no := true, false
	projectedTokens := map[string]bool{}
	keptVolumes := make([]corev1.Volume, 0, len(pod.Spec.Volumes))
	for _, volume := range pod.Spec.Volumes {
		if volume.Projected != nil {
			for _, projection := range volume.Projected.Sources {
				if projection.ServiceAccountToken != nil {
					projectedTokens[volume.Name] = true
				}
			}
		}
		if !projectedTokens[volume.Name] {
			keptVolumes = append(keptVolumes, volume)
		}
	}
	pod.Spec.Volumes = keptVolumes
	stripTokenMounts := func(container *corev1.Container) {
		kept := make([]corev1.VolumeMount, 0, len(container.VolumeMounts))
		for _, mount := range container.VolumeMounts {
			if !projectedTokens[mount.Name] {
				kept = append(kept, mount)
			}
		}
		container.VolumeMounts = kept
	}
	for index := range pod.Spec.Containers {
		stripTokenMounts(&pod.Spec.Containers[index])
	}
	for index := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[index]
		stripTokenMounts(container)
		if container.Image != service.Config.ExecdImage {
			return admissionError("unapproved workspace initializer")
		}
		container.SecurityContext = &corev1.SecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}
	}
	pod.Spec.AutomountServiceAccountToken = &no
	pod.Spec.ShareProcessNamespace = &no
	pod.Spec.SecurityContext = &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, FSGroup: &uid, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}
	ioOnly := pod.Labels["argus.io/workspace-mode"] == "io"
	approved := ""
	if !ioOnly {
		profileID, err := uuid.Parse(pod.Labels["argus.io/runtime-profile-id"])
		if err != nil {
			return admissionError("workspace runtime identity missing")
		}
		runtime, err := service.Sandbox.WorkspaceRuntimeByID(r.Context(), profileID)
		if err != nil {
			return admissionError("workspace profile unavailable")
		}
		if runtime.Identity().Label() != pod.Labels["argus.io/runtime-version"] {
			return admissionError("workspace runtime revision changed")
		}
		approved = runtime.Image.ImageRef + "@" + runtime.Image.Digest
	}
	for index := range pod.Spec.Containers {
		container := &pod.Spec.Containers[index]
		if container.Name == "egress" {
			if service.Config.EgressImage == "" || container.Image != service.Config.EgressImage {
				return admissionError("unapproved workspace network enforcer")
			}
			root := int64(0)
			// The user code shares the network namespace, so the policy API must
			// require an unexposed token even though its overlay cannot be relaxed.
			tokenFound := false
			for _, entry := range container.Env {
				if entry.Name == "OPENSANDBOX_EGRESS_TOKEN" && len(entry.Value) >= 32 {
					tokenFound = true
				}
			}
			if !tokenFound {
				container.Env = append(container.Env, corev1.EnvVar{Name: "OPENSANDBOX_EGRESS_TOKEN", Value: uuid.NewString()})
			}
			container.SecurityContext = &corev1.SecurityContext{RunAsUser: &root, RunAsNonRoot: &no, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"NET_ADMIN"}}}
			continue
		} // Fixed upstream network enforcer; never the user container.
		if container.Image != approved && container.Image != service.Config.IOImage {
			return admissionError("unapproved workspace image")
		}
		container.SecurityContext = &corev1.SecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}
		if container.Image == approved {
			if len(container.Command) == 0 {
				return admissionError("workspace runtime entrypoint missing")
			}
			for _, mount := range container.VolumeMounts {
				if mount.Name == "workspace-tls" {
					return admissionError("workspace credentials cannot be mounted into user code")
				}
			}
			managerUID := int64(10001)
			container.SecurityContext = &corev1.SecurityContext{RunAsUser: &managerUID, RunAsGroup: &managerUID, RunAsNonRoot: &yes, ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"SETUID", "SETGID", "KILL"}}}
			container.Command = []string{"/usr/local/bin/argus-workspace-supervisor"}
			container.Args = nil
			container.Env = append(service.Kubernetes.IOEnv(workspace), corev1.EnvVar{Name: "ARGUS_WORKSPACE_USER_UID", Value: fmt.Sprint(workspace.RuntimeUid)}, corev1.EnvVar{Name: "ARGUS_WORKSPACE_PROCESS_LIMIT", Value: fmt.Sprint(service.Config.ProcessLimit)})
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "workspace-tls", MountPath: "/var/run/argus/manager/tls", ReadOnly: true})
			container.ReadinessProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intPort(8448)}}, PeriodSeconds: 1}
		}
	}
	if !ioOnly {
		mode := int32(0440)
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: "workspace-tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: service.Config.TLSSecret, DefaultMode: &mode}}})
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: "argus-workspace-io", Image: service.Config.IOImage, Command: []string{"/usr/local/bin/argus-worker", "--pool=workspace-io"},
			Env: service.Kubernetes.IOEnv(workspace), Ports: []corev1.ContainerPort{{Name: "file-rpc", ContainerPort: 8447}},
			SecurityContext: &corev1.SecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
			Resources:       service.Kubernetes.IOResources(),
			VolumeMounts:    []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}, {Name: "workspace-tls", MountPath: "/var/run/argus/workspace", ReadOnly: true}},
			ReadinessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intPort(8447)}}, PeriodSeconds: 1, FailureThreshold: 60}})
		// Writable runtime scratch is explicit and separate from persistent files.
		for _, scratch := range []struct{ name, path string }{{"workspace-tmp", "/tmp"}, {"workspace-home", "/home/argus"}, {"workspace-logs", "/var/log"}} {
			limit := resource.MustParse("128Mi")
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: scratch.name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory, SizeLimit: &limit}}})
			for index := range pod.Spec.Containers {
				if pod.Spec.Containers[index].Image == approved {
					pod.Spec.Containers[index].VolumeMounts = append(pod.Spec.Containers[index].VolumeMounts, corev1.VolumeMount{Name: scratch.name, MountPath: scratch.path})
				}
			}
		}
	}
	return nil
}
