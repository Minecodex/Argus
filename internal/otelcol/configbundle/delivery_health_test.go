package configbundle

import (
	"testing"

	"github.com/google/uuid"
)

func TestMandatoryDeliveryHealthPreservesFailureClassificationWithoutSelfCollection(t *testing.T) {
	for _, resource := range []string{"host", "kubernetes_cluster"} {
		input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(),
			ConfigRevision: 1, ResourceType: resource, Role: "direct", Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct",
			EnrollmentEndpoint: "https://example.test/enroll", IngestGRPCEndpoint: "grpcs://example.test:4317", IngestHTTPEndpoint: "https://example.test:4318"}
		bundle, err := Render(input)
		if err != nil {
			t.Fatal(err)
		}
		targets := []string{"host"}
		if resource == "kubernetes_cluster" {
			targets = []string{"kubernetes_gateway", "kubernetes_agent"}
		}
		for _, target := range targets {
			config := decodeTarget(t, bundle, target)
			if nestedMap(t, config, "service", "telemetry", "metrics")["level"] != "detailed" {
				t.Fatal("native shutdown/retry error dimensions were lost")
			}
			identity := nestedMap(t, config, "extensions", "argus_identity")
			if target == "kubernetes_agent" {
				if identity["delivery_persistent_exporters"] != nil {
					t.Fatal("memory queue claimed durable replay")
				}
				continue
			}
			persistent := identity["delivery_persistent_exporters"].([]any)
			if len(persistent) != 1 || persistent[0] != "otlp/argus" || nestedMap(t, config, "exporters", "otlp/argus", "sending_queue")["storage"] != "file_storage" {
				t.Fatal("reporter durable scope diverged from the native exporter")
			}
		}
	}
}
