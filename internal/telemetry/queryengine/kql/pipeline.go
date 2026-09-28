package kql

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pipeline is the validated execution order, not an unordered bag of options.
// Unsupported reorderings are rejected before execution, never approximated.
type Pipeline struct {
	Context                  *LogContext
	Parser                   string
	Filters                  []Expr
	Unwrap                   string
	Aggregate                *Aggregation
	SortField, SortDirection string
	Limit                    int
	ExplicitLimit            bool
}

type Aggregation struct {
	Function, Field, Group string
	Bucket                 time.Duration
}

// SplitStages recognizes separators only outside quoted strings. It is also
// used for a full DSL document so UI and AI cannot disagree about its stages.
func SplitStages(input string) ([]string, error) {
	if len(input) > 65536 {
		return nil, fmt.Errorf("KQL document exceeds length limit")
	}
	var result []string
	start := 0
	quoted, escaped := false, false
	for i, r := range input {
		if escaped {
			escaped = false
			continue
		}
		if quoted && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if r == '|' && !quoted {
			result = append(result, strings.TrimSpace(input[start:i]))
			start = i + 1
		}
	}
	if quoted {
		return nil, fmt.Errorf("unterminated pipeline string")
	}
	result = append(result, strings.TrimSpace(input[start:]))
	if len(result) > 64 {
		return nil, fmt.Errorf("too many KQL stages")
	}
	return result, nil
}

var aggregateSyntax = regexp.MustCompile(`(?i)^(count|sum|avg|min|max|p95)\(([^()]*)\)(?:\s+by\s+(.+))?$`)
var bucketSyntax = regexp.MustCompile(`(?i)^bin\(timestamp,\s*([0-9]+(?:ms|s|m|h))\)$`)

func ParsePipeline(input string, maxRows int) (Pipeline, error) {
	p := Pipeline{Limit: maxRows, SortField: "timestamp", SortDirection: "DESC"}
	if maxRows <= 0 {
		return p, ErrBudget
	}
	stages, err := SplitStages(input)
	if err != nil {
		return p, err
	}
	phase := 0 // parser, filters/unwrap, aggregate, sort, limit
	for _, stage := range stages {
		if stage == "" {
			if len(stages) == 1 {
				continue
			}
			return p, fmt.Errorf("empty pipeline stage")
		}
		lower := strings.ToLower(stage)
		if p.Context != nil {
			return p, fmt.Errorf("context must be the final pipeline stage")
		}
		switch {
		case strings.HasPrefix(lower, "context "):
			if phase > 1 || p.Unwrap != "" {
				return p, fmt.Errorf("context cannot follow aggregation, sort, limit or unwrap")
			}
			value, e := ParseContext(stage)
			if e != nil {
				return p, e
			}
			if value.Before+value.After+1 > maxRows {
				return p, ErrBudget
			}
			p.Context = &value
			p.Limit = value.Before + value.After + 1
			p.ExplicitLimit = true
		case strings.HasPrefix(lower, "parse "):
			if phase != 0 || p.Parser != "" {
				return p, fmt.Errorf("parse must occur once before filters")
			}
			value := strings.TrimSpace(stage[6:])
			switch strings.ToLower(value) {
			case "json", "logfmt":
				p.Parser = strings.ToLower(value)
			default:
				if !strings.HasPrefix(strings.ToLower(value), "pattern ") {
					return p, fmt.Errorf("unsupported parser")
				}
				pattern, err := strconv.Unquote(strings.TrimSpace(value[8:]))
				if err != nil || pattern == "" || len(pattern) > 2048 {
					return p, fmt.Errorf("invalid pattern")
				}
				p.Parser = "pattern:" + pattern
			}
		case strings.HasPrefix(lower, "where "):
			if phase > 1 {
				return p, fmt.Errorf("where after aggregate, sort or limit is unsupported")
			}
			phase = 1
			filter, err := Parse(strings.TrimSpace(stage[6:]))
			if err != nil {
				return p, err
			}
			compiler := Compiler{Parser: p.Parser}
			if _, err := compiler.Compile(filter); err != nil {
				return p, err
			}
			p.Filters = append(p.Filters, filter)
		case strings.HasPrefix(lower, "unwrap "):
			if phase > 1 || p.Unwrap != "" {
				return p, fmt.Errorf("unwrap must occur once before aggregation")
			}
			phase = 1
			p.Unwrap = strings.TrimSpace(stage[7:])
			if strings.HasPrefix(p.Parser, "pattern:") {
				if !strings.HasPrefix(p.Unwrap, "pattern.") || len(p.Unwrap) <= 8 {
					return p, fmt.Errorf("pattern unwrap requires a field")
				}
			} else if _, _, ok := allowedFieldForParser(p.Unwrap, p.Parser); !ok {
				return p, fmt.Errorf("unsupported unwrap field")
			}
		case strings.HasPrefix(lower, "stats "):
			if phase > 1 {
				return p, fmt.Errorf("stats must occur once before sort and limit")
			}
			phase = 2
			p.Aggregate, err = parseAggregation(strings.TrimSpace(stage[6:]), p.Parser)
			if err != nil {
				return p, err
			}
			p.SortField = "count"
			if p.Aggregate.Function != "count" {
				p.SortField = "value"
			}
			if p.Aggregate.Bucket > 0 {
				p.SortField = "timestamp"
				p.SortDirection = "ASC"
			}
		case strings.HasPrefix(lower, "sort "):
			if phase >= 3 {
				return p, fmt.Errorf("sort must occur once before limit")
			}
			phase = 3
			parts := strings.Fields(lower)
			if len(parts) != 3 || (parts[2] != "asc" && parts[2] != "desc") {
				return p, fmt.Errorf("invalid sort")
			}
			p.SortField = parts[1]
			p.SortDirection = strings.ToUpper(parts[2])
			allowed := p.SortField == "timestamp" && (p.Aggregate == nil || p.Aggregate.Bucket > 0)
			if p.Aggregate != nil {
				allowed = allowed || p.SortField == "count" && p.Aggregate.Function == "count" || p.SortField == "value" && p.Aggregate.Function != "count" || p.SortField == "group_value" && p.Aggregate.Group != ""
			}
			if !allowed {
				return p, fmt.Errorf("sort field is not present in result")
			}
		case strings.HasPrefix(lower, "limit "):
			if phase >= 4 {
				return p, fmt.Errorf("limit must occur once at the end")
			}
			phase = 4
			p.Limit, err = strconv.Atoi(strings.TrimSpace(stage[6:]))
			p.ExplicitLimit = true
			if err != nil || p.Limit < 1 {
				return p, fmt.Errorf("invalid pipeline limit")
			}
			if p.Limit > maxRows {
				return p, ErrBudget
			}
		default:
			return p, fmt.Errorf("unsupported pipeline stage %q", stage)
		}
	}
	return p, nil
}

func parseAggregation(input, parser string) (*Aggregation, error) {
	parts := aggregateSyntax.FindStringSubmatch(input)
	if parts == nil {
		return nil, fmt.Errorf("unsupported aggregation")
	}
	a := &Aggregation{Function: strings.ToLower(parts[1]), Field: strings.TrimSpace(parts[2])}
	if a.Function == "count" {
		if a.Field != "" {
			return nil, fmt.Errorf("count() takes no field")
		}
	} else {
		if _, _, ok := allowedFieldForParser(a.Field, parser); !ok {
			return nil, fmt.Errorf("aggregation field is not queryable")
		}
	}
	group := strings.TrimSpace(parts[3])
	if strings.HasPrefix(strings.ToLower(group), "bin(") {
		end := strings.IndexByte(group, ')')
		if end < 0 {
			return nil, fmt.Errorf("invalid time bucket")
		}
		bucket := bucketSyntax.FindStringSubmatch(group[:end+1])
		if bucket == nil {
			return nil, fmt.Errorf("invalid time bucket")
		}
		var err error
		a.Bucket, err = time.ParseDuration(bucket[1])
		if err != nil || a.Bucket < time.Millisecond || a.Bucket > 24*time.Hour {
			return nil, fmt.Errorf("time bucket outside supported range")
		}
		group = strings.TrimSpace(group[end+1:])
		if group != "" {
			if !strings.HasPrefix(group, ",") {
				return nil, fmt.Errorf("invalid grouping")
			}
			group = strings.TrimSpace(group[1:])
			if group == "" {
				return nil, fmt.Errorf("missing grouping field")
			}
		}
	}
	if group != "" {
		if _, _, ok := allowedFieldForParser(group, parser); !ok {
			return nil, fmt.Errorf("group field is not queryable")
		}
		a.Group = group
	}
	return a, nil
}
