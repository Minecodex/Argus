package dashboard

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func parameterName(value string) string {
	match := queryParameter.FindStringSubmatch(value)
	if match == nil {
		return ""
	}
	if match[1] != "" {
		return match[1]
	}
	return match[2]
}

func matcherForSelection(name string, kind labels.MatchType, selection Selection) (*labels.Matcher, error) {
	if selection.All {
		if name == "__name__" {
			return labels.NewMatcher(labels.MatchRegexp, name, ".+")
		}
		return nil, nil
	}
	if len(selection.Values) == 0 {
		return nil, fmt.Errorf("%w: empty parameter selection", ErrInvalid)
	}
	if len(selection.Values) == 1 && (kind == labels.MatchEqual || kind == labels.MatchNotEqual) {
		return labels.NewMatcher(kind, name, selection.Values[0])
	}
	values := make([]string, len(selection.Values))
	for i, value := range selection.Values {
		values[i] = regexp.QuoteMeta(value)
	}
	if kind == labels.MatchEqual || kind == labels.MatchRegexp {
		kind = labels.MatchRegexp
	} else {
		kind = labels.MatchNotRegexp
	}
	return labels.NewMatcher(kind, name, "^(?:"+strings.Join(values, "|")+")$")
}

func bindPromQL(expression string, parameters map[string]Selection) (string, error) {
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil {
		return "", err
	}
	var bindErr error
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		selector, ok := node.(*parser.VectorSelector)
		if !ok {
			return nil
		}
		matchers := make([]*labels.Matcher, 0, len(selector.LabelMatchers))
		for _, matcher := range selector.LabelMatchers {
			name := parameterName(matcher.Value)
			if name == "" {
				matchers = append(matchers, matcher)
				continue
			}
			selection, ok := parameters[name]
			if !ok {
				bindErr = fmt.Errorf("%w: missing query parameter %s", ErrInvalid, name)
				return bindErr
			}
			bound, e := matcherForSelection(matcher.Name, matcher.Type, selection)
			if e != nil {
				bindErr = e
				return e
			}
			if bound != nil {
				matchers = append(matchers, bound)
			}
		}
		selector.LabelMatchers = matchers
		return nil
	})
	if bindErr != nil {
		return "", bindErr
	}
	bound := expr.String()
	if _, err := parser.NewParser(parser.Options{}).ParseExpr(bound); err != nil {
		return "", err
	}
	return bound, nil
}
