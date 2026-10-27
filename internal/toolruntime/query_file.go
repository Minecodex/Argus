package toolruntime

import "github.com/google/uuid"

// NativeQueryFile binds a provider-authored tool identity to one immutable
// result. A model cannot choose the name or assert file provenance.
func NativeQueryFile(toolID string, callID uuid.UUID) string {
	if callID == uuid.Nil {
		return ""
	}
	signal := ""
	switch toolID {
	case "telemetry.promql.query":
		signal = "metrics"
	case "telemetry.kql.query":
		signal = "logs"
	case "telemetry.skywalking.trace":
		signal = "traces"
	default:
		return ""
	}
	return "queries/" + signal + "/" + callID.String() + ".json"
}
