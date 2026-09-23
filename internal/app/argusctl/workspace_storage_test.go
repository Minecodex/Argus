package argusctl

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLockedWorkspaceChartResourceNames(t *testing.T) {
	archive := os.Getenv("ARGUS_RAWFILE_CHART_TEST_PATH")
	if archive == "" {
		t.Skip("requires the pinned RawFile chart archive")
	}
	digest, err := fileDigest(archive)
	if err != nil || digest != rawfileChartHash {
		t.Fatalf("pinned chart verification: %s %v", digest, err)
	}
	chart, err := loader.Load(archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m10-query-p5-m10-20260920-b", strings.Repeat("a", 53)} {
		cfg := &InstallConfig{}
		cfg.Spec.ReleaseID = id
		cfg.Spec.Workspace.Enabled = true
		if err := cfg.Spec.Workspace.validate(); err != nil {
			t.Fatal(err)
		}
		configuration := action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil)))
		install := action.NewInstall(configuration)
		install.ReleaseName = cfg.upstreamReleaseName("workspace-storage")
		install.Namespace = cfg.Spec.Workspace.StorageNamespace
		install.DryRunStrategy = action.DryRunClient
		install.KubeVersion, err = common.ParseKubeVersion("v1.36.1")
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := install.Run(chart, workspaceStorageValues(cfg.Spec.Workspace))
		if err != nil {
			t.Fatal(err)
		}
		accessor, err := release.NewAccessor(rendered)
		if err != nil {
			t.Fatal(err)
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(accessor.Manifest()), 4096)
		services := 0
		var driver *storagev1.CSIDriver
		var workloads []runtime.Object
		for {
			object := &unstructured.Unstructured{}
			if err := decoder.Decode(object); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if len(object.GetName()) > 63 {
				t.Fatalf("%s name exceeds DNS label limit: %s", object.GetKind(), object.GetName())
			}
			for key, value := range object.GetLabels() {
				if len(value) > 63 {
					t.Fatalf("label %s exceeds limit: %s", key, value)
				}
			}
			if object.GetKind() == "Service" {
				services++
			}
			// Helm adds these ownership annotations during installation; it
			// does not invent the version labels absent from this chart.
			object.SetAnnotations(map[string]string{"meta.helm.sh/release-name": install.ReleaseName, "meta.helm.sh/release-namespace": install.Namespace})
			switch object.GetKind() {
			case "CSIDriver":
				driver = &storagev1.CSIDriver{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, driver); err != nil {
					t.Fatal(err)
				}
			case "DaemonSet":
				object.SetNamespace(install.Namespace)
				set := &appsv1.DaemonSet{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, set); err != nil {
					t.Fatal(err)
				}
				workloads = append(workloads, set)
			}
		}
		if services == 0 {
			t.Fatal("node metrics Service was not rendered")
		}
		if driver == nil {
			t.Fatal("CSIDriver was not rendered")
		}
		if err := validateExistingRawfileDriver(context.Background(), fake.NewSimpleClientset(workloads...), driver); err != nil {
			t.Fatalf("pinned chart cannot be reinstalled: %v", err)
		}
	}
}
