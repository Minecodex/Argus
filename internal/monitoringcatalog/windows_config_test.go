package monitoringcatalog

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestWindowsNativeCollectorConfiguration(t *testing.T) {
	binary := os.Getenv("ARGUS_MONITORING_WINDOWS_COLLECTOR_BINARY")
	if binary == "" {
		t.Skip("requires the built Windows Collector")
	}
	iis, err := ReceiverConfig("iis", uuid.New(), []byte(`{"endpoint":"local"}`), []byte(`{}`), false)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{
		"receivers": map[string]any{
			"iis/plan8":             iis,
			"hostmetrics/plan8":     map[string]any{"collection_interval": "30s", "scrapers": map[string]any{"cpu": map[string]any{}, "memory": map[string]any{}, "disk": map[string]any{}, "filesystem": map[string]any{}, "network": map[string]any{}}},
			"windowseventlog/plan8": map[string]any{"channel": "Application", "start_at": "end"},
		},
		"exporters": map[string]any{"nop/plan8": map[string]any{}},
		"service": map[string]any{"pipelines": map[string]any{
			"metrics": map[string]any{"receivers": []string{"iis/plan8", "hostmetrics/plan8"}, "exporters": []string{"nop/plan8"}},
			"logs":    map[string]any{"receivers": []string{"windowseventlog/plan8"}, "exporters": []string{"nop/plan8"}},
		}},
	}
	raw, _ := json.Marshal(config)
	file := filepath.Join(t.TempDir(), "config.json")
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(t.Context(), binary, "validate", "--config="+file).CombinedOutput(); err != nil {
		t.Fatalf("Windows configuration validation: %v\n%s", err, output)
	}
}
