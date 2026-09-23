package workspace

import (
	"slices"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
)

// Validate mount sources independently of the upstream template merge. Image
// approval alone must not allow a template to expose control-plane credentials.
func (service Service) validatePodSources(pod *corev1.Pod, workspace db.Workspace) error {
	ioOnly := pod.Labels["argus.io/workspace-mode"] == "io"
	for _, volume := range pod.Spec.Volumes {
		switch {
		case volume.PersistentVolumeClaim != nil:
			if volume.Name != "workspace" || volume.PersistentVolumeClaim.ClaimName != workspace.PvcName {
				return admissionError("workspace volume ownership mismatch")
			}
		case volume.Secret != nil:
			if !ioOnly || volume.Name != "workspace-tls" || volume.Secret.SecretName != service.Config.TLSSecret {
				return admissionError("unapproved workspace credential volume")
			}
		case volume.EmptyDir != nil:
		case volume.Projected != nil:
			// Kubernetes may have injected its default service-account volume;
			// hardenPod removes it together with every corresponding mount.
			if !slices.ContainsFunc(volume.Projected.Sources, func(source corev1.VolumeProjection) bool { return source.ServiceAccountToken != nil }) {
				return admissionError("unapproved workspace projected volume")
			}
		default:
			return admissionError("unapproved workspace volume source")
		}
	}
	for _, container := range append(slices.Clone(pod.Spec.InitContainers), pod.Spec.Containers...) {
		if len(container.EnvFrom) != 0 {
			return admissionError("workspace environment imports are forbidden")
		}
		for _, env := range container.Env {
			if env.ValueFrom != nil && (env.ValueFrom.SecretKeyRef != nil || env.ValueFrom.ConfigMapKeyRef != nil) {
				return admissionError("workspace credential environment references are forbidden")
			}
		}
		for _, mount := range container.VolumeMounts {
			if mount.Name == "workspace-tls" && (!ioOnly || container.Image != service.Config.IOImage) {
				return admissionError("workspace credentials cannot be mounted into user code")
			}
		}
		if container.Image == service.Config.IOImage {
			if !ioOnly || !slices.Equal(container.Command, []string{"/usr/local/bin/argus-worker", "--pool=workspace-io"}) || len(container.Args) != 0 {
				return admissionError("workspace file role must use its fixed entrypoint")
			}
		}
	}
	return nil
}
