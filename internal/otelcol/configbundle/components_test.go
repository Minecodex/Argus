package configbundle

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestDistributionInventoryMatchesLockedOCBManifests(t *testing.T) {
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "otelcol", "builder-"+strings.ReplaceAll(platform, "_", "-")+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Dist struct {
				Version          string
				CollectorVersion string `json:"otelcol_version"`
			}
			Receivers, Processors, Exporters, Extensions []struct {
				GoMod string `json:"gomod"`
			}
		}
		if err := yaml.Unmarshal(raw, &config); err != nil {
			t.Fatal(err)
		}
		if config.Dist.Version != DistributionVersion || config.Dist.CollectorVersion != CollectorVersion {
			t.Fatal("OCB and catalog versions differ", platform)
		}
		ids := map[string]bool{}
		for _, group := range [][]struct {
			GoMod string `json:"gomod"`
		}{config.Receivers, config.Processors, config.Exporters, config.Extensions} {
			for _, component := range group {
				module := filepath.Base(strings.Fields(component.GoMod)[0])
				aliases := map[string]string{"argusidentity": "argus_identity", "argusgatewayidentity": "argus_gateway_identity", "k8sclusterreceiver": "k8s_cluster", "memorylimiterprocessor": "memory_limiter", "filestorage": "file_storage", "healthcheckextension": "health_check"}
				id := aliases[module]
				if id == "" {
					id = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(module, "receiver"), "processor"), "exporter")
				}
				ids[id] = true
			}
		}
		actual := []string{}
		for id := range ids {
			actual = append(actual, id)
		}
		slices.Sort(actual)
		if !slices.Equal(actual, DistributionComponents(platform)) {
			t.Fatalf("%s inventory differs from OCB: %v vs %v", platform, actual, DistributionComponents(platform))
		}
	}
}
