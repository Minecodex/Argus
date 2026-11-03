package telemetry

import "strconv"

// Methods describe the existing query engine's actual storage representation.
// They are not measurements, filled invocation arguments or new permissions.
type ObservationMetricMethods struct {
	Representation  string `json:"representation"`
	CountExpression string `json:"count_expression"`
	SumExpression   string `json:"sum_expression"`
	MeanExpression  string `json:"mean_expression"`
	CountUnit       string `json:"count_unit"`
	MeanUnit        string `json:"mean_unit"`
	Notes           string `json:"notes"`
}

// ObservationMethodsForMetric describes the supported methods for a verified
// catalog type. Unknown and ordinary scalar types have no histogram methods.
func ObservationMethodsForMetric(metric MetricDescriptor) *ObservationMetricMethods {
	selector := func(name string) string { return "{__name__=" + strconv.Quote(name) + "}" }
	methods := &ObservationMetricMethods{CountUnit: "{observation}", MeanUnit: metric.Unit,
		Notes: "Use the same authorized source_keys, label scope and original positive time window with metric/query. These expressions describe query representation, not observed values. Count is observations; mean uses the original measurement unit. Mean is not P95. Exact-name metrics may collide with classic suffixes: inspect actual input metadata and labels, never merge incompatible type/unit variants. An empty result from an incompatible representation does not prove the histogram has no observations."}
	methods.Notes += " PromQL can execute arithmetic or aggregation across float series with incompatible units or Gauge/Counter semantics; execution success does not validate units. Do not claim that the engine always rejects such semantic mixtures."
	switch metric.Type {
	case "histogram":
		methods.Representation = "classic_histogram"
		methods.CountExpression = selector(metric.Name + "_count")
		methods.SumExpression = selector(metric.Name + "_sum")
		methods.Notes += " Classic explicit histogram: use the _count/_sum/_bucket family; native histogram_count(base)/histogram_sum(base) and the direct base selector do not produce these classic values."
	case "exponential_histogram":
		methods.Representation = "native_histogram"
		methods.CountExpression = "histogram_count(" + selector(metric.Name) + ")"
		methods.SumExpression = "histogram_sum(" + selector(metric.Name) + ")"
		methods.Notes += " Native exponential histogram: use native histogram functions on the base histogram; do not assume classic suffix series exist."
	case "summary":
		methods.Representation = "summary"
		methods.CountExpression = selector(metric.Name + "_count")
		methods.SumExpression = selector(metric.Name + "_sum")
		methods.Notes += " Summary count/sum: do not invent histogram buckets or a percentile that was not exported."
	default:
		return nil
	}
	methods.MeanExpression = "(" + methods.SumExpression + ") / (" + methods.CountExpression + ")"
	return methods
}
