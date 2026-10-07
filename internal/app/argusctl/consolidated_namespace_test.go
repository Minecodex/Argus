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

func TestConsolidatedChartsHaveUniqueResourcesAndStableNamespaceOwnership(t *testing.T) {
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "local-formal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.Namespaces.Observability != cfg.Spec.Namespaces.System || cfg.Spec.Workspace.StorageNamespace != cfg.Spec.Namespaces.System {
		t.Fatal("formal profile is not consolidated")
	}
	cred := localHardeningTestCredentials()
	network := NetworkProfile{}
	network.Policy.APISupported = true
	charts := []struct {
		name, namespace string
		values          map[string]any
	}{
		{"argus-foundation", "default", foundationValues(cfg, network)},
		{"argus-data", cfg.Spec.Namespaces.System, dataValues(cfg, cred, network)},
		{"argus-platform", cfg.Spec.Namespaces.System, withTestHTTPSInternalAddress(platformValues(cfg, cred, "setup", "idempotency", "cursor", "pending", "0123456789abcdef0123456789abcdef", network))},
		{"argus-sandbox", cfg.Spec.Namespaces.Sandbox, sandboxValues(cfg, "test-key")},
		{"argus-telemetry-pipeline", cfg.Spec.Namespaces.System, telemetryValues(cfg, network)},
	}
	seen := map[string]string{}
	namespaces := 0
	for _, item := range charts {
		ch, err := loadLocalChart(root, item.name)
		if err != nil {
			t.Fatal(err)
		}
		install := action.NewInstall(action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil))))
		install.ReleaseName = item.name
		install.Namespace = item.namespace
		install.DryRunStrategy = action.DryRunClient
		r, err := install.Run(ch, item.values)
		if err != nil {
			t.Fatal(err)
		}
		accessor, err := release.NewAccessor(r)
		if err != nil {
			t.Fatal(err)
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(accessor.Manifest()), 4096)
		for {
			obj := &unstructured.Unstructured{}
			if err := decoder.Decode(obj); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if obj.GetKind() == "" {
				continue
			}
			key := obj.GetAPIVersion() + "/" + obj.GetKind() + "/" + obj.GetNamespace() + "/" + obj.GetName()
			if previous := seen[key]; previous != "" {
				t.Fatalf("resource collision %s between %s and %s", key, previous, item.name)
			}
			seen[key] = item.name
			if obj.GetKind() == "Namespace" {
				namespaces++
				if obj.GetName() == cfg.Spec.Namespaces.System && obj.GetLabels()["argus.io/plane"] != "system" {
					t.Fatal("merged namespace lost system ownership")
				}
			}
		}
	}
	if namespaces != 2 {
		t.Fatalf("expected system and sandbox namespaces, got %d", namespaces)
	}
}

func TestNamespaceDefaultsAndSandboxIsolation(t *testing.T) {
	root, _ := findRepoRoot(".")
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "local-formal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Namespaces.Observability = ""
	cfg.Spec.Workspace.StorageNamespace = ""
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.Namespaces.Observability != cfg.Spec.Namespaces.System || cfg.Spec.Workspace.StorageNamespace != cfg.Spec.Namespaces.System {
		t.Fatal("default namespace mismatch")
	}
	cfg.Spec.Workspace.StorageNamespace = cfg.Spec.Namespaces.Sandbox
	if err := cfg.Validate(); err == nil {
		t.Fatal("privileged driver accepted in sandbox namespace")
	}
	cfg.Spec.Workspace.StorageNamespace = cfg.Spec.Namespaces.System
	cfg.Spec.Namespaces.Sandbox = cfg.Spec.Namespaces.System
	if err := cfg.Validate(); err == nil {
		t.Fatal("sandbox accepted in control namespace")
	}
}
