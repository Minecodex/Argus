package kql

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Scope struct {
	SourceKeys   []string
	EnterpriseID uuid.UUID
	ResourceIDs  []uuid.UUID
}
type Budget struct {
	MaxRows      int
	MaxScanBytes int64
	Timeout      time.Duration
}
type Request struct {
	Expression, Pipeline string
	Start, End           time.Time
	Scope                Scope
	Budget               Budget
}
type Result struct {
	LatestSampleAt *time.Time
	ResultType     string
	Columns        []Column
	Partial        bool
	Data           []map[string]any
	Warnings       []string
	Elapsed        time.Duration
	ScannedBytes   int64
	ScannedRows    int64
}

type TableRouter interface {
	Table(string, uuid.UUID) (string, error)
}

type Op string

const (
	OpEqual    Op = "="
	OpNotEqual Op = "!="
	OpGT       Op = ">"
	OpGTE      Op = ">="
	OpLT       Op = "<"
	OpLTE      Op = "<="
	OpContains Op = ":"
	OpExists   Op = "exists"
)

type Expr interface{ exprNode() }
type Predicate struct {
	Field  string
	Op     Op
	Value  string
	Quoted bool
}

func (Predicate) exprNode() {}

type BoolExpr struct {
	Left  Expr
	Op    string
	Right Expr
}

func (BoolExpr) exprNode() {}

type NotExpr struct{ Inner Expr }

func (NotExpr) exprNode() {}

type Parser struct {
	tokens []string
	index  int
}

func Parse(input string) (Expr, error) {
	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}
	parser := &Parser{tokens: tokens}
	expr, err := parser.parseOr()
	if err != nil {
		return nil, err
	}
	if parser.index != len(tokens) {
		return nil, fmt.Errorf("unexpected token %q", tokens[parser.index])
	}
	return expr, nil
}
func (p *Parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek("OR") {
		p.index++
		right, e := p.parseAnd()
		if e != nil {
			return nil, e
		}
		left = BoolExpr{Left: left, Op: "OR", Right: right}
	}
	return left, nil
}
func (p *Parser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek("AND") {
		p.index++
		right, e := p.parseUnary()
		if e != nil {
			return nil, e
		}
		left = BoolExpr{Left: left, Op: "AND", Right: right}
	}
	return left, nil
}
func (p *Parser) parseUnary() (Expr, error) {
	if p.peek("NOT") {
		p.index++
		inner, e := p.parseUnary()
		return NotExpr{Inner: inner}, e
	}
	if p.peek("(") {
		p.index++
		inner, e := p.parseOr()
		if e != nil {
			return nil, e
		}
		if !p.peek(")") {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.index++
		return inner, nil
	}
	return p.parsePredicate()
}
func (p *Parser) parsePredicate() (Expr, error) {
	if p.index >= len(p.tokens) {
		return nil, fmt.Errorf("predicate expected")
	}
	field := p.tokens[p.index]
	p.index++
	if p.index >= len(p.tokens) {
		return nil, fmt.Errorf("operator expected")
	}
	op := Op(p.tokens[p.index])
	p.index++
	if strings.EqualFold(string(op), string(OpExists)) {
		return Predicate{Field: field, Op: OpExists}, nil
	}
	if p.index >= len(p.tokens) {
		return nil, fmt.Errorf("value expected")
	}
	value := p.tokens[p.index]
	p.index++
	quoted := strings.HasPrefix(value, "\"")
	if quoted {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return nil, err
		}
		value = decoded
	}
	return Predicate{Field: field, Op: op, Value: value, Quoted: quoted}, nil
}
func (p *Parser) peek(value string) bool {
	return p.index < len(p.tokens) && strings.EqualFold(p.tokens[p.index], value)
}

type Compiler struct {
	Table  string
	Args   []any
	Parser string
}

func (c *Compiler) Compile(expr Expr) (string, error) {
	switch node := expr.(type) {
	case Predicate:
		return c.predicate(node)
	case BoolExpr:
		l, e := c.Compile(node.Left)
		if e != nil {
			return "", e
		}
		r, e := c.Compile(node.Right)
		if e != nil {
			return "", e
		}
		return "(" + l + " " + node.Op + " " + r + ")", nil
	case NotExpr:
		inner, e := c.Compile(node.Inner)
		if e != nil {
			return "", e
		}
		return "NOT (" + inner + ")", nil
	default:
		return "", fmt.Errorf("unsupported expression")
	}
}
func (c *Compiler) predicate(p Predicate) (string, error) {
	if p.Op == OpExists {
		return c.exists(p.Field)
	}
	field, key, ok := allowedFieldForParser(p.Field, c.Parser)
	if !ok {
		return "", fmt.Errorf("field %q is not queryable", p.Field)
	}
	if key != "" {
		c.Args = append(c.Args, key)
	}
	// ClickHouse may stringify non-string JSON scalars. Quoted KQL literals
	// retain their string type independently of server conversion settings.
	if p.Quoted && strings.HasPrefix(p.Field, "json.") {
		field = "if(JSONType(body, ?)='String', JSONExtractString(body, ?), NULL)"
		c.Args = append(c.Args, key)
	}
	switch p.Op {
	case OpContains:
		if strings.ContainsAny(p.Value, "*?") {
			var pattern strings.Builder
			pattern.WriteString("(?is)^")
			for _, r := range p.Value {
				switch r {
				case '*':
					pattern.WriteString(".*")
				case '?':
					pattern.WriteString(".")
				default:
					pattern.WriteString(regexp.QuoteMeta(string(r)))
				}
			}
			pattern.WriteString("$")
			c.Args = append(c.Args, pattern.String())
			return "match(" + field + ", ?)", nil
		}
		c.Args = append(c.Args, p.Value)
		return "positionCaseInsensitive(" + field + ", ?)>0", nil
	case OpEqual, OpNotEqual, OpGT, OpGTE, OpLT, OpLTE:
		var value any = p.Value
		if p.Field == "severity_number" {
			number, err := strconv.ParseUint(p.Value, 10, 8)
			if err != nil {
				return "", fmt.Errorf("severity_number requires an unsigned byte")
			}
			value = int64(number)
		} else if !p.Quoted && strings.HasPrefix(p.Field, "json.") && (p.Op == OpEqual || p.Op == OpNotEqual) {
			value = typedValue(p.Value)
			switch value.(type) {
			case int64, float64:
				field = "toFloat64OrNull(JSONExtractRaw(body, ?))"
			default:
				if p.Value == "true" || p.Value == "false" || p.Value == "null" {
					field = "JSONExtractRaw(body, ?)"
				}
			}
		} else if !p.Quoted && (p.Op == OpGT || p.Op == OpGTE || p.Op == OpLT || p.Op == OpLTE) && p.Field != "timestamp" {
			value = typedValue(p.Value)
			switch value.(type) {
			case int64, float64:
				if strings.HasPrefix(p.Field, "json.") {
					field = "JSONExtractRaw(body, ?)"
				}
				field = "toFloat64OrNull(toString(" + field + "))"
			default:
				return "", fmt.Errorf("numeric comparison requires a number")
			}
		}
		c.Args = append(c.Args, value)
		return field + " " + string(p.Op) + " ?", nil
	default:
		return "", fmt.Errorf("operator %q is unsupported", p.Op)
	}
}

func (c *Compiler) exists(field string) (string, error) {
	switch field {
	case "body", "severity_text", "service_name", "trace_id", "span_id":
		column, _, _ := allowedField(field)
		return "notEmpty(" + column + ")", nil
	case "timestamp", "severity_number":
		return "1", nil
	}
	for prefix, column := range map[string]string{
		"stream_labels.":       "stream_labels",
		"structured_metadata.": "structured_metadata",
		"resource_attributes.": "resource_attributes",
	} {
		if key, ok := strings.CutPrefix(field, prefix); ok && key != "" {
			c.Args = append(c.Args, key)
			return "mapContains(" + column + ", ?)", nil
		}
	}
	return "", fmt.Errorf("field %q is not queryable", field)
}
func allowedField(field string) (string, string, bool) {
	switch field {
	case "resource_id", "source_id":
		return "toString(" + field + ")", "", true
	case "source_type", "source_key", "event_id":
		return field, "", true
	case "source_revision":
		return "toString(source_revision)", "", true
	case "body":
		return "body", "", true
	case "timestamp":
		return "timestamp", "", true
	case "severity_text":
		return "severity_text", "", true
	case "severity_number":
		return "severity_number", "", true
	case "service_name":
		return "service_name", "", true
	case "trace_id":
		return "trace_id", "", true
	case "span_id":
		return "span_id", "", true
	default:
		if strings.HasPrefix(field, "stream_labels.") {
			return "stream_labels[?]", strings.TrimPrefix(field, "stream_labels."), true
		}
		if strings.HasPrefix(field, "structured_metadata.") {
			return "structured_metadata[?]", strings.TrimPrefix(field, "structured_metadata."), true
		}
		if strings.HasPrefix(field, "resource_attributes.") {
			return "resource_attributes[?]", strings.TrimPrefix(field, "resource_attributes."), true
		}
		return "", "", false
	}
}

func allowedFieldForParser(field, parser string) (string, string, bool) {
	if value, key, ok := allowedField(field); ok {
		return value, key, true
	}
	if parser == "json" && strings.HasPrefix(field, "json.") {
		return "JSONExtractString(body, ?)", strings.TrimPrefix(field, "json."), true
	}
	if parser == "logfmt" && strings.HasPrefix(field, "logfmt.") {
		return `extractKeyValuePairs(body, '=', ' \t\r\n', '"')[?]`, strings.TrimPrefix(field, "logfmt."), true
	}
	return "", "", false
}
func typedValue(value string) any {
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f
	}
	return value
}

func compilePipeline(pipeline, where, order string, limit int, args *[]any) (string, string, int, error) {
	where, order, limit, _, _, _, err := compilePipelineOptions(pipeline, where, order, limit, args)
	return where, order, limit, err
}

func compilePipelineOptions(pipeline, where, order string, limit int, args *[]any) (string, string, int, string, string, string, error) {
	p, err := ParsePipeline(pipeline, limit)
	if err != nil {
		return "", "", 0, "", "", "", err
	}
	for _, filter := range p.Filters {
		compiler := Compiler{Parser: p.Parser}
		compiled, err := compiler.Compile(filter)
		if err != nil {
			return "", "", 0, "", "", "", err
		}
		where += " AND (" + compiled + ")"
		*args = append(*args, compiler.Args...)
	}
	stats := ""
	if p.Aggregate != nil {
		stats = p.Aggregate.Group
	}
	return where, p.SortField + " " + p.SortDirection, p.Limit, p.Parser, p.Unwrap, stats, nil
}

func parseStructuredBody(body, parser string) map[string]string {
	result := map[string]string{}
	if parser == "json" {
		var values map[string]any
		if json.Unmarshal([]byte(body), &values) == nil {
			for key, value := range values {
				result[key] = fmt.Sprint(value)
			}
		}
	}
	if parser == "logfmt" {
		return parseLogfmt(body)
	}
	return result
}

func parsePipelineBody(body, parser string) map[string]string {
	if strings.HasPrefix(parser, "pattern:") {
		return parsePatternBody(body, strings.TrimPrefix(parser, "pattern:"))
	}
	return parseStructuredBody(body, parser)
}

// parsePatternBody supports the bounded KQL pattern form where captures are
// written as <name>, for example `request <method> <path> <status>`. It is
// intentionally a literal matcher and never becomes a user-provided SQL
// expression.
func parsePatternBody(body, pattern string) map[string]string {
	result := map[string]string{}
	var literals []string
	var names []string
	for {
		start := strings.IndexByte(pattern, '<')
		if start < 0 {
			literals = append(literals, pattern)
			break
		}
		end := strings.IndexByte(pattern[start+1:], '>')
		if end < 0 {
			return result
		}
		end += start + 1
		literals = append(literals, pattern[:start])
		name := strings.TrimSpace(pattern[start+1 : end])
		if name == "" || strings.ContainsAny(name, " <>\t\r\n") {
			return result
		}
		names = append(names, name)
		pattern = pattern[end+1:]
	}
	if len(names) == 0 || len(literals) != len(names)+1 {
		return result
	}
	position := 0
	for i, name := range names {
		literal := literals[i]
		if literal != "" {
			index := strings.Index(body[position:], literal)
			if index < 0 {
				return result
			}
			position += index + len(literal)
		}
		next := len(body)
		if literals[i+1] != "" {
			index := strings.Index(body[position:], literals[i+1])
			if index < 0 {
				return result
			}
			next = position + index
		}
		result[name] = body[position:next]
		position = next
	}
	return result
}
