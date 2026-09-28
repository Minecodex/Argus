package argusctl

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/engine"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestTelemetryChartRendersAllMigrationsAndOrderedExecution(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	chart, err := loadLocalChart(root, "argus-telemetry-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	values, err := chartutil.ToRenderValues(chart, map[string]any{"namespace": "test-observability", "releaseId": "test-release"}, common.ReleaseOptions{Name: "test-release", Namespace: "test-observability", IsInstall: true}, common.DefaultCapabilities)
	if err != nil {
		t.Fatal(err)
	}
	files, err := engine.Render(chart, values)
	if err != nil {
		t.Fatal(err)
	}
	manifest := files["argus-telemetry-pipeline/templates/migration.yaml"]
	migrations, err := filepath.Glob(filepath.Join(root, "migrations", "clickhouse", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	configFound, jobFound := false, false
	for {
		var raw runtime.RawExtension
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		var config corev1.ConfigMap
		if err := yaml.Unmarshal(raw.Raw, &config); err != nil {
			t.Fatal(err)
		}
		if config.Kind == "ConfigMap" {
			configFound = true
			if len(config.Data) != len(migrations) || !strings.Contains(config.Data["00002_source_identity.sql"], "SELECT 4") {
				t.Fatal("rendered chart omitted the source migration", config.Data)
			}
		}
		if config.Kind == "Job" {
			jobFound = true
			var job batchv1.Job
			if err := yaml.Unmarshal(raw.Raw, &job); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(job.Spec.Template.Spec.Containers[0].Args[0], "for migration in /migrations/*.sql") {
				t.Fatal("migration job executes only one version")
			}
		}
	}
	if !configFound || !jobFound {
		t.Fatal("migration resources were not rendered")
	}
}
