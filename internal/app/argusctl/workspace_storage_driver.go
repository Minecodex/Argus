package argusctl

import (
	"context"
	"fmt"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const rawfileReleaseDigest = "sha256:eb99a5caf5b2a87471b0fba4d67e310aae4fe06b663eb9c85bfa96f49a545446"

// RawFile v0.15.1's CSIDriver and DaemonSet do not render version/instance
// labels. Helm adds release annotations; verify that installation's actual
// driver container instead of requiring metadata the pinned chart never emits.
func validateExistingRawfileDriver(ctx context.Context, client kubernetes.Interface, driver *storagev1.CSIDriver) error {
	releaseName := driver.Annotations["meta.helm.sh/release-name"]
	namespace := driver.Annotations["meta.helm.sh/release-namespace"]
	if driver.Name != "rawfile.csi.openebs.io" || !dnsLabel.MatchString(releaseName) || !dnsLabel.MatchString(namespace) {
		return fmt.Errorf("existing RawFile driver installation identity is unavailable")
	}
	sets, err := client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("inspect existing RawFile workload: %w", err)
	}
	matched := 0
	for _, set := range sets.Items {
		if set.Annotations["meta.helm.sh/release-name"] != releaseName || set.Annotations["meta.helm.sh/release-namespace"] != namespace {
			continue
		}
		for _, container := range set.Spec.Template.Spec.Containers {
			if container.Name != "csi-driver" {
				continue
			}
			matched++
			image := strings.TrimPrefix(container.Image, "docker.io/")
			if image != "openebs/rawfile-localpv:v0.15.1" && image != "openebs/rawfile-localpv:v0.15.1@"+rawfileReleaseDigest && image != "openebs/rawfile-localpv@"+rawfileReleaseDigest {
				return fmt.Errorf("existing RawFile workload does not use the approved v0.15.1 image")
			}
			provisionerMatches := false
			for _, value := range container.Env {
				if value.Name == "PROVISIONER_NAME" && value.Value == driver.Name && value.ValueFrom == nil {
					provisionerMatches = true
				}
			}
			if !provisionerMatches {
				return fmt.Errorf("existing RawFile workload provisioner identity differs")
			}
		}
	}
	if matched != 1 {
		return fmt.Errorf("existing RawFile installation requires exactly one matching driver workload")
	}
	return nil
}
