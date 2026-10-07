package configbundle

import (
	"encoding/json"
	"testing"
)

// The upstream scraper defaults omit these gauges used by our host templates.
func TestHostBasicEnablesDashboardUtilizationMetrics(t *testing.T) {
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		t.Run(platform, func(t *testing.T) {
			bundle, err := Render(RenderInput{CollectorID: "collector", ResourceID: "host", ResourceType: "host", Role: "direct", Platform: platform,
				RouteKind: "direct_argus", Transport: "direct", ProfileKeys: []string{"host-basic"},
				EnrollmentEndpoint: "https://api.example.test/enroll", IngestGRPCEndpoint: "grpcs://ingest.example.test:4317", IngestHTTPEndpoint: "https://ingest.example.test:4318"})
			if err != nil {
				t.Fatal(err)
			}
			var decoded Bundle
			if err := json.Unmarshal(bundle, &decoded); err != nil {
				t.Fatal(err)
			}
			var host map[string]any
			if err := json.Unmarshal(decoded.Host, &host); err != nil {
				t.Fatal(err)
			}
			scrapers := host["receivers"].(map[string]any)["hostmetrics"].(map[string]any)["scrapers"].(map[string]any)
			for scraper, metric := range map[string]string{"cpu": "system.cpu.utilization", "memory": "system.memory.utilization"} {
				metricConfig := scrapers[scraper].(map[string]any)["metrics"].(map[string]any)[metric].(map[string]any)
				if metricConfig["enabled"] != true {
					t.Fatalf("%s disabled on %s", metric, platform)
				}
			}
			if _, exists := scrapers["load"]; exists != (platform != "windows_amd64") {
				t.Fatal("load scraper platform support changed")
			}
		})
	}
}
