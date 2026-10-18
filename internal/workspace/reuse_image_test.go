package workspace

import (
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func TestWarmWorkspaceRequiresCurrentFileHelperImage(t *testing.T) {
	for _, name := range []string{"workspace-io", "argus-workspace-io"} {
		for _, image := range []string{"registry/backend@sha256:old", "registry/backend@sha256:current", ""} {
			pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: image}}}}
			if got := workspaceIOImageCurrent(pod, "registry/backend@sha256:current"); got != (image == "registry/backend@sha256:current") {
				t.Fatalf("%s/%s accepted=%v", name, image, got)
			}
		}
	}
	if workspaceIOImageCurrent(&corev1.Pod{}, "registry/backend@sha256:current") || workspaceIOImageCurrent(nil, "registry/backend@sha256:current") {
		t.Fatal("missing file helper accepted")
	}
}
