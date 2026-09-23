package argusctl

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestExistingRawfileDriverUsesWorkloadIdentity(t *testing.T) {
	for _, test := range []struct {
		name, image, owner, provisioner       string
		missingIdentity, duplicate, wantError bool
	}{
		{name: "upstream without version labels", image: "docker.io/openebs/rawfile-localpv:v0.15.1", owner: "storage", provisioner: "rawfile.csi.openebs.io"},
		{name: "pinned digest", image: "openebs/rawfile-localpv@" + rawfileReleaseDigest, owner: "storage", provisioner: "rawfile.csi.openebs.io"},
		{name: "wrong version despite claimed label", image: "openebs/rawfile-localpv:v0.14.0", owner: "storage", provisioner: "rawfile.csi.openebs.io", wantError: true},
		{name: "wrong digest", image: "openebs/rawfile-localpv:v0.15.1@sha256:other", owner: "storage", provisioner: "rawfile.csi.openebs.io", wantError: true},
		{name: "foreign release", image: "openebs/rawfile-localpv:v0.15.1", owner: "another-release", provisioner: "rawfile.csi.openebs.io", wantError: true},
		{name: "different provisioner", image: "openebs/rawfile-localpv:v0.15.1", owner: "storage", provisioner: "another.csi.io", wantError: true},
		{name: "no installation identity", missingIdentity: true, wantError: true},
		{name: "ambiguous workloads", image: "openebs/rawfile-localpv:v0.15.1", owner: "storage", provisioner: "rawfile.csi.openebs.io", duplicate: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := &storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: "rawfile.csi.openebs.io", Annotations: map[string]string{"meta.helm.sh/release-name": "storage", "meta.helm.sh/release-namespace": "storage-system"}}}
			if test.missingIdentity {
				driver.Annotations = nil
			}
			if test.name == "wrong version despite claimed label" {
				driver.Labels = map[string]string{"app.kubernetes.io/version": "v0.15.1"}
			}
			set := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "driver", Namespace: "storage-system", Annotations: map[string]string{"meta.helm.sh/release-name": test.owner, "meta.helm.sh/release-namespace": "storage-system"}}, Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "csi-driver", Image: test.image, Env: []corev1.EnvVar{{Name: "PROVISIONER_NAME", Value: test.provisioner}}}}}}}}
			objects := []runtime.Object{set}
			if test.duplicate {
				second := set.DeepCopy()
				second.Name = "second-driver"
				objects = append(objects, second)
			}
			if err := validateExistingRawfileDriver(context.Background(), fake.NewSimpleClientset(objects...), driver); (err != nil) != test.wantError {
				t.Fatalf("validation error: %v", err)
			}
		})
	}
}
