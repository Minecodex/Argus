package dashboard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/prometheus/prometheus/model/labels"
)

var queryFieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,255}$`)

func compileLogFilter(f Filter) (string, error) {
	if !queryFieldName.MatchString(f.Field) {
		return "", fmt.Errorf("invalid log field")
	}
	values := f.Values
	if len(values) == 0 {
		values = []string{f.Value}
	}
	if f.Field == "severity_number" {
		for _, v := range values {
			if _, err := strconv.ParseUint(v, 10, 8); err != nil {
				return "", err
			}
		}
	}
	predicate := kql.Predicate{Field: f.Field, Op: kql.Op(f.Operator), Value: "$input", Quoted: !numericFilter(f)}
	bound, err := bindKQLPredicate(predicate, map[string]Selection{"input": {Values: values}}, true)
	return bound.text, err
}

func compileMetricFilter(f Filter) (string, error) {
	if f.Operator != "=" && f.Operator != "!=" && f.Operator != "=~" && f.Operator != "!~" {
		return "", fmt.Errorf("invalid label operator")
	}
	if len(f.Values) == 0 {
		return f.Field + f.Operator + strconv.Quote(f.Value), nil
	}
	kind := labels.MatchEqual
	switch f.Operator {
	case "=":
	case "!=":
		kind = labels.MatchNotEqual
	case "=~":
		kind = labels.MatchRegexp
	case "!~":
		kind = labels.MatchNotRegexp
	default:
		return "", fmt.Errorf("invalid label operator")
	}
	m, err := matcherForSelection(f.Field, kind, Selection{Values: f.Values})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(m.String()), nil
}
