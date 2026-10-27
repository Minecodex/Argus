package configbundle

// This mandatory loopback reader serves only the identity control reporter.
// Optional collector_metrics controls export/query visibility independently.
func addMandatoryDeliveryHealth(config map[string]any) {
	service := config["service"].(map[string]any)
	telemetry, ok := service["telemetry"].(map[string]any)
	if !ok {
		telemetry = map[string]any{}
		service["telemetry"] = telemetry
	}
	telemetry["metrics"] = map[string]any{"level": "normal", "readers": []map[string]any{{"pull": map[string]any{"exporter": map[string]any{"prometheus": map[string]any{"host": "127.0.0.1", "port": 8888}}}}}}
}
