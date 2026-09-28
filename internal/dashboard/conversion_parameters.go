package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/printer"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/prometheus/prometheus/promql/parser"
)

// Parameter names are local to a query. Normalize them by input identity,
// explicit mapping and native type when proving equivalence, never by value.
func equivalentConversionTargets(a, b Target) (bool, error) {
	first, err := normalizedConversionParameters(a)
	if err != nil {
		return false, err
	}
	second, err := normalizedConversionParameters(b)
	if err != nil {
		return false, err
	}
	return equivalentDSL(a.Language, first, second)
}
func normalizedConversionParameters(target Target) (DSL, error) {
	query := *target.SourceDefinition.DSL
	names := map[string]string{}
	types := map[string]ast.Type{}
	if target.Language == queryengine.LanguageTrace {
		var err error
		types, err = graphQLParameterTypes(query)
		if err != nil {
			return query, err
		}
	}
	for _, binding := range target.ParameterBindings {
		name := binding.Parameter
		binding.Parameter = ""
		typ := ""
		if types[name] != nil {
			typ = fmt.Sprint(printer.Print(types[name]))
		}
		raw, _ := json.Marshal(struct {
			Binding ParameterBinding
			Type    string
		}{binding, typ})
		hash := sha256.Sum256(raw)
		names[name] = "p" + hex.EncodeToString(hash[:])[:62]
	}
	switch target.Language {
	case queryengine.LanguagePromQL:
		expr, err := parser.NewParser(parser.Options{}).ParseExpr(query.Expression)
		if err != nil {
			return query, err
		}
		parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
			if selector, ok := node.(*parser.VectorSelector); ok {
				for _, m := range selector.LabelMatchers {
					if name := names[parameterName(m.Value)]; name != "" {
						m.Value = "$" + name
					}
				}
			}
			return nil
		})
		query.Expression = expr.String()
	case queryengine.LanguageKQL:
		stages, err := kql.SplitStages(query.Expression)
		if err != nil {
			return query, err
		}
		if query.Pipeline != "" {
			extra, err := kql.SplitStages(query.Pipeline)
			if err != nil {
				return query, err
			}
			stages = append(stages, extra...)
		}
		// Reuse the binder with symbolic values; numeric fields must retain their
		// syntax rather than trying to parse a symbolic name as a number.
		for i, stage := range stages {
			if i == 0 && stage == "*" {
				continue
			}
			if i > 0 && strings.HasPrefix(strings.ToLower(stage), "context ") {
				context, err := kql.ParseContext(stage)
				if err != nil {
					return query, err
				}
				if name := names[parameterName(context.EventID)]; name != "" {
					context.EventID = "$" + name
				}
				stages[i] = kql.FormatContext(context)
				continue
			}
			prefix := ""
			if i > 0 {
				if !strings.HasPrefix(strings.ToLower(stage), "where ") {
					continue
				}
				prefix = "where "
				stage = stage[6:]
			}
			expr, err := kql.Parse(stage)
			if err != nil {
				return query, err
			}
			var render func(kql.Expr) string
			render = func(expr kql.Expr) string {
				switch node := expr.(type) {
				case kql.Predicate:
					if name := names[parameterName(node.Value)]; name != "" {
						node.Value = "$" + name
					}
					return formatKQLPredicate(node)
				case kql.BoolExpr:
					return "(" + render(node.Left) + " " + node.Op + " " + render(node.Right) + ")"
				case kql.NotExpr:
					return "NOT (" + render(node.Inner) + ")"
				}
				return ""
			}
			stages[i] = prefix + render(expr)
		}
		query.Expression = stages[0]
		query.Pipeline = ""
		for i := 1; i < len(stages); i++ {
			if i > 1 {
				query.Pipeline += " | "
			}
			query.Pipeline += stages[i]
		}
	case queryengine.LanguageTrace:
		doc, op, root, err := graphDocument(query.Expression)
		if err != nil {
			return query, err
		}
		var replace func(ast.Value)
		replace = func(value ast.Value) {
			switch v := value.(type) {
			case *ast.Variable:
				if name := names[v.Name.Value]; name != "" {
					v.Name.Value = name
				}
			case *ast.ObjectValue:
				for _, f := range v.Fields {
					replace(f.Value)
				}
			case *ast.ListValue:
				for _, v := range v.Values {
					replace(v)
				}
			}
		}
		for _, arg := range root.Arguments {
			replace(arg.Value)
		}
		definitions := []*ast.VariableDefinition{}
		seen := map[string]bool{}
		for _, definition := range op.VariableDefinitions {
			if name := names[definition.Variable.Name.Value]; name != "" {
				definition.Variable.Name.Value = name
			}
			key := fmt.Sprint(printer.Print(definition))
			if !seen[key] {
				definitions = append(definitions, definition)
				seen[key] = true
			}
		}
		op.VariableDefinitions = definitions
		query.Expression = fmt.Sprint(printer.Print(doc))
	}
	return query, nil
}
