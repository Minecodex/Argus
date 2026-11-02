package telemetry

import (
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestObservedMetricMethodsKeepClassicNativeAndUnknownDistinct(t *testing.T) {
	for _, item := range []struct{ kind, representation, count string }{
		{"histogram", "classic_histogram", `{__name__="duration_count"}`},
		{"exponential_histogram", "native_histogram", `histogram_count({__name__="duration"})`},
		{"summary", "summary", `{__name__="duration_count"}`},
	} {
		metric := MetricDescriptor{Name: "duration", Type: item.kind, Unit: "s"}
		methods := ObservationMethodsForMetric(metric)
		if methods == nil || methods.Representation != item.representation || methods.CountExpression != item.count || methods.CountUnit != "{observation}" || methods.MeanUnit != "s" {
			t.Fatal("actual metric representation/method/result units changed", methods)
		}
		for _, expression := range []string{methods.CountExpression, methods.SumExpression, methods.MeanExpression} {
			if _, err := parser.NewParser(parser.Options{}).ParseExpr(expression); err != nil {
				t.Fatal("catalog emitted invalid PromQL", expression, err)
			}
		}
	}
	for _, kind := range []string{"unknown", "mixed", "counter", "gauge", ""} {
		if ObservationMethodsForMetric(MetricDescriptor{Name: "unknown", Type: kind}) != nil {
			t.Fatal("unverified/ordinary type fabricated histogram methods", kind)
		}
	}
	name := "duration\"} or vector(999) #"
	methods := ObservationMethodsForMetric(MetricDescriptor{Name: name, Type: "histogram", Unit: "s"})
	expression, err := parser.NewParser(parser.Options{}).ParseExpr(methods.CountExpression)
	if err != nil {
		t.Fatal(err)
	}
	selector, ok := expression.(*parser.VectorSelector)
	if !ok || len(selector.LabelMatchers) != 1 || selector.LabelMatchers[0].Value != name+"_count" || strings.Contains(methods.MeanUnit, "999") {
		t.Fatal("metric name escaped a safe exact-name selector", methods)
	}
}
