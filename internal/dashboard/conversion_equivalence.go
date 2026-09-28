package dashboard

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/printer"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/prometheus/prometheus/promql/parser"
)

func equivalentDSL(language queryengine.Language, a, b DSL) (bool, error) {
	if a.Operation != b.Operation || !reflect.DeepEqual(a.Variables, b.Variables) {
		return false, nil
	}
	switch language {
	case queryengine.LanguagePromQL:
		if a.Pipeline != "" || b.Pipeline != "" {
			return false, nil
		}
		first, err := parser.NewParser(parser.Options{}).ParseExpr(a.Expression)
		if err != nil {
			return false, err
		}
		second, err := parser.NewParser(parser.Options{}).ParseExpr(b.Expression)
		if err != nil {
			return false, err
		}
		for _, expr := range []parser.Expr{first, second} {
			parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
				if selector, ok := node.(*parser.VectorSelector); ok {
					for _, matcher := range selector.LabelMatchers {
						if name := parameterName(matcher.Value); name != "" {
							matcher.Value = "$" + name
						}
					}
				}
				return nil
			})
		}
		// The parser retains offsets, @ modifiers, matching rules and histogram modifiers.
		return unparen(first).String() == unparen(second).String(), nil
	case queryengine.LanguageKQL:
		return equivalentLogQuery(a, b)
	case queryengine.LanguageTrace:
		first, err := canonicalGraphQuery(a)
		if err != nil {
			return false, err
		}
		second, err := canonicalGraphQuery(b)
		return first == second, err
	default:
		return false, fmt.Errorf("unsupported language")
	}
}
func canonicalGraphQuery(query DSL) (string, error) {
	doc, op, _, err := graphDocument(query.Expression)
	if err != nil {
		return "", err
	}
	var values func(ast.Value)
	values = func(value ast.Value) {
		switch v := value.(type) {
		case *ast.ObjectValue:
			slices.SortFunc(v.Fields, func(a, b *ast.ObjectField) int { return strings.Compare(a.Name.Value, b.Name.Value) })
			for _, field := range v.Fields {
				values(field.Value)
			}
		case *ast.ListValue:
			for _, value := range v.Values {
				values(value)
			}
		}
	}
	var selection func(*ast.SelectionSet) error
	selection = func(set *ast.SelectionSet) error {
		if set == nil {
			return nil
		}
		for _, item := range set.Selections {
			field, ok := item.(*ast.Field)
			if !ok {
				return fmt.Errorf("fragments are not representable")
			}
			if field.Alias != nil || len(field.Directives) > 0 {
				return fmt.Errorf("aliases or directives are not representable")
			}
			slices.SortFunc(field.Arguments, func(a, b *ast.Argument) int { return strings.Compare(a.Name.Value, b.Name.Value) })
			for _, arg := range field.Arguments {
				values(arg.Value)
			}
			if err := selection(field.SelectionSet); err != nil {
				return err
			}
		}
		slices.SortFunc(set.Selections, func(a, b ast.Selection) int {
			return strings.Compare(a.(*ast.Field).Name.Value, b.(*ast.Field).Name.Value)
		})
		return nil
	}
	if err := selection(op.SelectionSet); err != nil {
		return "", err
	}
	slices.SortFunc(op.VariableDefinitions, func(a, b *ast.VariableDefinition) int {
		return strings.Compare(a.Variable.Name.Value, b.Variable.Name.Value)
	})
	return fmt.Sprint(printer.Print(doc)), nil
}
