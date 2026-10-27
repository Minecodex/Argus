package argusctl

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
)

func TestFrozenCRDHookImageRendersInLockedUpstreamChart(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", ".cache", "charts", "altinity-0.27.3.tgz")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("requires locked upstream chart cache")
	}
	chart, err := loader.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	reference := "registry.example:5001/argus/argus-install-tools:verified-build@sha256:" + strings.Repeat("a", 64)
	repository, tag, err := pinnedCRDHookImage(reference)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"crdHook": map[string]any{"enabled": true, "image": map[string]any{"repository": repository, "tag": tag, "pullPolicy": "IfNotPresent"}}}
	configuration := action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil)))
	install := action.NewInstall(configuration)
	install.ReleaseName = "owned-crd-pin"
	install.Namespace = "owned-namespace"
	install.DryRunStrategy = action.DryRunClient
	rendered, err := install.Run(chart, values)
	if err != nil {
		t.Fatal(err)
	}
	accessor, err := release.NewAccessor(rendered)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, hook := range accessor.Hooks() {
		value, err := release.NewHookAccessor(hook)
		if err != nil {
			t.Fatal(err)
		}
		body := value.Manifest()
		if !strings.Contains(body, "name: crd-install") {
			continue
		}
		found = true
		if !strings.Contains(body, reference) || strings.Contains(body, "bitnami/kubectl:latest") || !strings.Contains(body, "kubectl apply --server-side=true --force-conflicts") {
			t.Fatal("upstream CRD hook lost frozen image or original behavior")
		}
	}
	if !found {
		t.Fatal("locked chart did not render its actual CRD hook")
	}
}
