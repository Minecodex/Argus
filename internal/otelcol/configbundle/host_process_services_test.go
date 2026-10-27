package configbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHostProcessAndServiceConfigurationsKeepScopeAndHideCommandArguments(t *testing.T) {
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		in := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), ResourceType: "host", Role: "direct", Platform: platform, SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test"}
		service := "host_systemd_metrics"
		component := "systemd"
		sourceType := "host_systemd"
		if platform == "windows_amd64" {
			service, component, sourceType = "host_windows_service_metrics", "windows_service", "host_windows_services"
		}
		for _, key := range []string{"host_process_metrics", service} {
			in.Collections = append(in.Collections, Collection{ID: uuid.NewString(), Integration: key, TargetID: in.ResourceID, TargetType: "host", Config: json.RawMessage(`{}`), Enabled: true, Revision: 1, AssignmentEpoch: 1})
		}
		raw, err := Render(in)
		if err != nil {
			t.Fatal(err)
		}
		config := decodeTarget(t, raw, "host")
		receivers := nestedMap(t, config, "receivers")
		for key, raw := range receivers {
			if strings.HasPrefix(key, "hostmetrics/") {
				r := raw.(map[string]any)
				scrapers := r["scrapers"].(map[string]any)
				p := scrapers["process"].(map[string]any)
				attrs := p["resource_attributes"].(map[string]any)
				for _, name := range []string{"process.command_line", "process.command", "process.executable.path", "process.owner"} {
					if attrs[name].(map[string]any)["enabled"] != false {
						t.Fatalf("%s exposed %s", platform, name)
					}
				}
				if platform == "windows_amd64" && scrapers["processes"] != nil {
					t.Fatal("unsupported process-count scraper enabled on Windows")
				}
			}
		}
		if receivers[component+"/collection_"+strings.ReplaceAll(in.Collections[1].ID, "-", "")] == nil {
			t.Fatal("wrong platform service receiver")
		}
		sources, err := Sources(raw)
		if err != nil || len(sources) != 2 {
			t.Fatalf("independent sources: %v %v", sources, err)
		}
		seen := map[string]bool{}
		for _, source := range sources {
			seen[source.Type] = true
			if source.ResourceID != in.ResourceID || source.CollectionID == "" {
				t.Fatal("lost host responsibility")
			}
		}
		if !seen["host_processes"] || !seen[sourceType] {
			t.Fatal("sources not split by capability")
		}
		in.Collections[1].Enabled = false
		raw, err = Render(in)
		if err != nil {
			t.Fatal(err)
		}
		sources, _ = Sources(raw)
		if len(sources) != 1 || sources[0].Type != "host_processes" {
			t.Fatal("service toggle disabled processes")
		}
		in.Collections[0].Enabled = false
		raw, err = Render(in)
		if err != nil {
			t.Fatal(err)
		}
		sources, _ = Sources(raw)
		if len(sources) != 0 {
			t.Fatal("all-off still collected host data")
		}
	}
}
