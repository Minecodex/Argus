package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

func TestFrontendCollectorRegistryMatchesDistribution(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "packages", "api-client", "src", "generated", "collector-registry.ts"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)export const collectorComponents = (\{.*?\}) as const;`).FindSubmatch(raw)
	if len(match) != 2 {
		t.Fatal("missing generated Collector registry")
	}
	var actual map[string][]string
	if err := json.Unmarshal(match[1], &actual); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		if !slices.Equal(actual[platform], configbundle.DistributionComponents(platform)) {
			t.Fatal("frontend Collector inventory differs", platform)
		}
	}
	for name, value := range map[string]string{"collectorDistributionVersion": configbundle.DistributionVersion, "collectorVersion": configbundle.CollectorVersion, "collectorConfigSchemaVersion": configbundle.CatalogConfigSchemaVersion} {
		if !strings.Contains(string(raw), "export const "+name+" = "+strconv.Quote(value)+";") {
			t.Fatal("frontend Collector version differs", name)
		}
	}
}
