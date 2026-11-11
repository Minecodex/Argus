package promqlengine

import "testing"

func TestScalarPairsRequireBothRequestedOperandsWithSamePredicates(t *testing.T) {
	for expression, count := range map[string]int{
		`sum(rate(latency_sum{verb!="WATCH"}[5m])) / sum(rate(latency_count{verb!="WATCH"}[5m]))`: 1,
		`sum(latency_sum)+sum(latency_count)`:                                                     1,
		`latency_sum{service="a"}+latency_count{service="b"}`:                                     0,
		`latency_sum`:                  0,
		`latency_count`:                0,
		`latency_bucket+latency_count`: 0,
		`other_sum+latency_count`:      0,
	} {
		if got := len(scalarPairSelectors(expression)); got != count {
			t.Fatalf("%s: scalar pairs=%d want=%d", expression, got, count)
		}
	}
}

func TestPairedScalarViewsKeepIndependentCursorsAndMetricNames(t *testing.T) {
	items := []*series{{labels: labelSet(map[string]string{"__name__": "latency_sum"})}, {labels: labelSet(map[string]string{"__name__": "latency_count"})}}
	sum := &metricFilteredSet{SeriesSet: &seriesSet{items: items, index: -1}, name: "latency_sum"}
	count := &metricFilteredSet{SeriesSet: &seriesSet{items: items, index: -1}, name: "latency_count"}
	if !count.Next() || count.At().Labels().Get("__name__") != "latency_count" || !sum.Next() || sum.At().Labels().Get("__name__") != "latency_sum" || sum.Next() || count.Next() {
		t.Fatal("paired views shared cursors or exposed a sibling metric")
	}
}
