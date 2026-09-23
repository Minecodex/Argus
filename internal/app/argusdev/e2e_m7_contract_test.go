package argusdev

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestM7CollectorRequestsMatchCurrentContract(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "telemetryapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema := document.Components.Schemas["CollectorPreview"].Value
	for _, action := range []string{"install", "configure", "repair", "upgrade", "uninstall"} {
		for _, image := range []string{"", "registry.test/argus-otelcol:fixture"} {
			body := m7CollectorPreviewBody("11111111-1111-4111-8111-111111111111", []string{"22222222-2222-4222-8222-222222222222"}, image)
			if action != "install" {
				body["expected_version"] = 7
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			if err := schema.VisitJSON(wire); err != nil {
				t.Fatalf("%s preview violates contract: %v", action, err)
			}
			delete(wire, "transport")
			if err := schema.VisitJSON(wire); err == nil {
				t.Fatal("missing transport was accepted")
			}
		}
	}
}
