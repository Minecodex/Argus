package telemetry

import (
	"regexp"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestMetricDiscoveryExamplesParseAsBoundedNameSelectors(t *testing.T) {
	pattern := regexp.MustCompile(`\{__name__[^}]+\}`)
	queryParser := parser.NewParser(parser.Options{})
	examples := telemetryDiscovery["telemetry.promql.query"].Examples
	count := 0
	for _, example := range examples {
		for _, expression := range pattern.FindAllString(example.Guidance, -1) {
			node, err := queryParser.ParseExpr(expression)
			if err != nil {
				t.Fatalf("discovery guidance contains an invalid selector: %v", err)
			}
			selector, ok := node.(*parser.VectorSelector)
			if !ok || len(selector.LabelMatchers) != 1 || selector.LabelMatchers[0].Name != "__name__" || selector.LabelMatchers[0].Type != labels.MatchRegexp || selector.LabelMatchers[0].Matches("") {
				t.Fatal("name discovery example must retain a non-empty metric name matcher")
			}
			count++
		}
	}
	if count != 2 {
		t.Fatal("both language examples must expose the executable discovery selector")
	}
}
