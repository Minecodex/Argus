package dashboard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

func TestDashboardAcceptsRenderedApplicationSDKCapabilities(t *testing.T) {
	for _, protocol := range []string{"otlp", "jaeger"} {
		t.Run(protocol, func(t *testing.T) {
			signals := []string{"traces"}
			if protocol == "otlp" {
				signals = []string{"metrics", "logs", "traces"}
			}
			input := configbundle.RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 1, ResourceType: "host", Role: "direct", Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://api.example.test/enroll", IngestGRPCEndpoint: "grpcs://ingest.example.test:4317", IngestHTTPEndpoint: "https://ingest.example.test:4318", ApplicationEndpoints: []configbundle.ApplicationEndpoint{{ID: uuid.NewString(), ApplicationID: uuid.NewString(), Protocol: protocol, Signals: signals, Version: 1}}}
			bundle, err := configbundle.Render(input)
			if err != nil {
				t.Fatal(err)
			}
			sources, err := configbundle.Sources(bundle)
			if err != nil {
				t.Fatal(err)
			}
			matched := 0
			for _, source := range sources {
				if source.IngestEndpointID == input.ApplicationEndpoints[0].ID {
					matched++
					if !sourceCapabilityMatches(source.Type, source.ResourceType, source.CapabilityVersion, "v1") {
						t.Fatalf("production SDK source rejected by dashboard capability: %+v", source)
					}
				}
			}
			if matched != 1 {
				t.Fatalf("expected one real application source definition, got %d", matched)
			}
		})
	}
}

func TestSDKCapabilityMappingRejectsOtherIdentityAndVersionCombinations(t *testing.T) {
	for _, tc := range [][4]string{
		{"otlp", "host", "application/v1", "v1"},
		{"filelog", "application", "application/v1", "v1"},
		{"otlp", "application", "application/v2", "v1"},
		{"otlp", "application", "application/v1", "v2"},
		{"jaeger", "monitoring_target", "application/v1", "v1"},
	} {
		if sourceCapabilityMatches(tc[0], tc[1], tc[2], tc[3]) {
			t.Fatalf("unsupported capability admitted: %v", tc)
		}
	}
	if !sourceCapabilityMatches("redis", "monitoring_target", "v1", "v1") {
		t.Fatal("existing native capability stopped matching")
	}
}
