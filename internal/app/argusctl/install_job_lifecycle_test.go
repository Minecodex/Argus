package argusctl

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// A successful initializer can finish before another image has downloaded.
// Helm's aggregate readiness watcher and the installer must still be able to
// observe that completion when the slower workload becomes ready.
func TestInstallerReadinessJobsSurviveSlowSiblingReadiness(t *testing.T) {
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"evaluation", "local-hardening"} {
		t.Run(profile, func(t *testing.T) {
			cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", profile+".yaml"))
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]map[string]any{
				"argus-data":               dataValues(cfg, localHardeningTestCredentials()),
				"argus-platform":           withTestHTTPSInternalAddress(platformValues(cfg, localHardeningTestCredentials(), "setup", "idempotency", "cursor", "pending", "0123456789abcdef0123456789abcdef")),
				"argus-telemetry-pipeline": telemetryValues(cfg),
			}
			want := map[string]bool{"argus-minio-bucket-init": false, "argus-postgresql-migration": false, "argus-telemetry-catalog-sync": false, "argus-clickhouse-telemetry-migration": false}
			for name, chartValues := range values {
				chart, err := loadLocalChart(root, name)
				if err != nil {
					t.Fatal(err)
				}
				install := action.NewInstall(action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil))))
				install.ReleaseName, install.Namespace = "argus-job-lifecycle", cfg.Spec.Namespaces.System
				install.DryRunStrategy = action.DryRunClient
				rendered, err := install.Run(chart, chartValues)
				if err != nil {
					t.Fatal(err)
				}
				accessor, err := release.NewAccessor(rendered)
				if err != nil {
					t.Fatal(err)
				}
				manifest := accessor.Manifest()
				for _, hook := range accessor.Hooks() {
					value, err := release.NewHookAccessor(hook)
					if err != nil {
						t.Fatal(err)
					}
					manifest += "\n---\n" + value.Manifest()
				}
				decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(manifest), 4096)
				for {
					var object unstructured.Unstructured
					if err := decoder.Decode(&object); err == io.EOF {
						break
					} else if err != nil {
						t.Fatal(err)
					}
					if _, expected := want[object.GetName()]; object.GetKind() != "Job" || !expected {
						continue
					}
					want[object.GetName()] = true
					if _, exists, err := unstructured.NestedFieldNoCopy(object.Object, "spec", "ttlSecondsAfterFinished"); exists || err != nil {
						t.Fatalf("%s can disappear before aggregate readiness: %v", object.GetName(), err)
					}
					if object.GetName() == "argus-telemetry-catalog-sync" && object.GetAnnotations()["helm.sh/hook-delete-policy"] != "before-hook-creation" {
						t.Fatal("catalog hook must be replaced on the next reconciliation")
					}
				}
			}
			for name, found := range want {
				if !found {
					t.Fatalf("required readiness Job %s was not rendered", name)
				}
			}
		})
	}
}
