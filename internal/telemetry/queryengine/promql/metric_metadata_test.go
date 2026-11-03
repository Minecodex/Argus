package promqlengine

import "testing"

func TestOnlyOriginalSelectorsMayUseInputMetadataAsOutputSemantics(t *testing.T) {
	for _, expression := range []string{"network_bytes", `network_bytes{direction="receive"}`, "((network_bytes))", "network_bytes offset 1m"} {
		if !originalMetricSelector(expression) {
			t.Fatal("original selector rejected", expression)
		}
	}
	for _, expression := range []string{"rate(network_bytes[5m])", "sum(network_bytes)", "network_bytes * 2", `label_replace(rate(network_bytes[5m]),"__name__","network_bytes","","")`, "vector(1)"} {
		if originalMetricSelector(expression) {
			t.Fatal("derived expression was given source unit/type authority", expression)
		}
	}
}
