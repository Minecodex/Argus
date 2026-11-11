package promqlengine

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// Pair reuse is local to one expression/Querier and only activated when both
// scalar operands are actually requested with the same label predicates.
func scalarPairSelectors(expression string) map[string]bool {
	node, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil {
		return nil
	}
	seen := map[string]uint8{}
	parser.Inspect(node, func(node parser.Node, _ []parser.Node) error {
		if selector, ok := node.(*parser.VectorSelector); ok {
			_, metric, key := scalarPairMatcher(selector.LabelMatchers)
			if key != "" {
				if strings.HasSuffix(metric, "_sum") {
					seen[key] |= 1
				} else {
					seen[key] |= 2
				}
			}
		}
		return nil
	})
	out := map[string]bool{}
	for key, flags := range seen {
		if flags == 3 {
			out[key] = true
		}
	}
	return out
}

func scalarPairMatcher(matchers []*labels.Matcher) ([]*labels.Matcher, string, string) {
	for i, matcher := range matchers {
		if matcher == nil || matcher.Name != "__name__" || matcher.Type != labels.MatchEqual {
			continue
		}
		metric := matcher.Value
		base := metric
		if strings.HasSuffix(metric, "_count") {
			base = strings.TrimSuffix(metric, "_count")
		} else if strings.HasSuffix(metric, "_sum") {
			base = strings.TrimSuffix(metric, "_sum")
		}
		if base == metric || base == "" {
			return nil, "", ""
		}
		normalized := append([]*labels.Matcher(nil), matchers...)
		normalized[i] = labels.MustNewMatcher(labels.MatchEqual, "__name__", base+"_sum")
		return normalized, metric, selectionKey(true, nil, normalized)
	}
	return nil, "", ""
}

type metricFilteredSet struct {
	storage.SeriesSet
	name string
}

func (s *metricFilteredSet) Next() bool {
	for s.SeriesSet.Next() {
		if s.SeriesSet.At().Labels().Get("__name__") == s.name {
			return true
		}
	}
	return false
}
