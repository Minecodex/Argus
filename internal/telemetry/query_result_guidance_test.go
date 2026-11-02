package telemetry

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/mcp"
)

func TestNativeSignalDiscoveryUsesOneOriginalConsumptionRule(t *testing.T) {
	for _, name := range []string{"telemetry.promql.query", "telemetry.kql.query", "telemetry.skywalking.trace"} {
		description := telemetryDiscovery[name].ResultDescription
		if !strings.Contains(description, modelOriginalAnalysisGuidance) || strings.Contains(description, "must read") || strings.Contains(description, "Read the file.") {
			t.Fatal("native signal discovery contradicts certified small-result consumption", name)
		}
	}
}

func TestTelemetryDiscoveryFitsProductionSchema(t *testing.T) {
	for name, discovery := range telemetryDiscovery {
		t.Run(name, func(t *testing.T) {
			if _, _, err := mcp.NormalizeDiscovery(discovery); err != nil {
				t.Fatal("production discovery validation failed", err)
			}
		})
	}
}
