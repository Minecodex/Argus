package presentation

import (
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
)

// Input metadata describes an output quantity only for an original metric and
// a single unambiguous name/source descriptor. Derived expressions keep their
// existing explicit display policy; never inherit their inputs' physical units.
func metricDisplayDescriptor(data, metric map[string]any) (metricmeta.Descriptor, bool) {
	meta, _ := data["argus_meta"].(map[string]any)
	if meta["result_is_original_metric"] != true || meta["input_metrics_complete"] != true || meta["input_metrics_origin"] != metricmeta.Origin {
		return metricmeta.Descriptor{}, false
	}
	name, _ := metric["__name__"].(string)
	source, _ := metric["argus_source_id"].(string)
	if source == "" {
		source, _ = metric["source_id"].(string)
	}
	if name == "" || source == "" {
		return metricmeta.Descriptor{}, false
	}
	rows, _ := meta["input_metrics"].([]any)
	matched, found := 0, metricmeta.Descriptor{}
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok || row["name"] != name {
			continue
		}
		key, _ := row["source_key"].(string)
		id, _, _ := strings.Cut(key, ":")
		if id != source {
			continue
		}
		matched++
		found = metricmeta.Descriptor{Name: name, SourceKey: key}
		found.Unit, _ = row["unit"].(string)
		found.Type, _ = row["type"].(string)
	}
	return found, matched == 1
}
