package configbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestApplicationAcknowledgementChainsHaveNoVolatileBatchBeforePersistentQueue(t *testing.T) {
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		t.Run(platform, func(t *testing.T) {
			input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 1, ResourceType: "host", Role: "direct", Platform: platform, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.example.test/v1/identity/enroll", IngestGRPCEndpoint: "grpcs://ingest.example.test:4317", IngestHTTPEndpoint: "https://ingest.example.test"}
			input.ApplicationEndpoints = []ApplicationEndpoint{{ID: uuid.NewString(), ApplicationID: uuid.NewString(), Protocol: "otlp", Signals: []string{"metrics", "logs", "traces"}, Version: 1}, {ID: uuid.NewString(), ApplicationID: uuid.NewString(), Protocol: "jaeger", Signals: []string{"traces"}, Version: 1}}
			bundle, err := Render(input)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := Extract(bundle, "host")
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if json.Unmarshal(raw, &config) != nil {
				t.Fatal("invalid rendered configuration")
			}
			queue := config["exporters"].(map[string]any)["otlp/argus"].(map[string]any)["sending_queue"].(map[string]any)
			if queue["storage"] != "file_storage" || queue["enabled"] != true {
				t.Fatal("SDK acknowledgement has no persistent queue")
			}
			pipelines := config["service"].(map[string]any)["pipelines"].(map[string]any)
			count := 0
			for name, value := range pipelines {
				if !strings.Contains(name, "/application_") {
					continue
				}
				count++
				identity, limiter := false, false
				for _, value := range value.(map[string]any)["processors"].([]any) {
					name := value.(string)
					if name == "batch" || strings.HasPrefix(name, "batch/") {
						t.Fatal("SDK request acknowledges a volatile batch buffer")
					}
					identity = identity || strings.HasPrefix(name, "argus_application_identity/")
					limiter = limiter || name == "memory_limiter"
				}
				if !identity || !limiter {
					t.Fatal("durability removed identity or memory admission checks")
				}
			}
			if count != 4 {
				t.Fatalf("missing OTLP/Jaeger signal path: %d", count)
			}
		})
	}
}
