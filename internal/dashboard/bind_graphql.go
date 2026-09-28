package dashboard

import (
	"fmt"
	"math"
	"strconv"

	"github.com/graphql-go/graphql/language/ast"
	graphqlparser "github.com/graphql-go/graphql/language/parser"
)

func graphQLParameterTypes(query DSL) (map[string]ast.Type, error) {
	document, err := graphqlparser.Parse(graphqlparser.ParseParams{Source: query.Expression})
	if err != nil {
		return nil, err
	}
	result := map[string]ast.Type{}
	operations := 0
	for _, definition := range document.Definitions {
		op, ok := definition.(*ast.OperationDefinition)
		if !ok {
			continue
		}
		if query.Operation != "" && (op.Name == nil || op.Name.Value != query.Operation) {
			continue
		}
		operations++
		for _, variable := range op.VariableDefinitions {
			result[variable.Variable.Name.Value] = variable.Type
		}
	}
	if operations != 1 {
		return nil, fmt.Errorf("%w: choose one GraphQL operation", ErrInvalid)
	}
	return result, nil
}

func graphQLSelection(typ ast.Type, selection Selection) (any, error) {
	if required, ok := typ.(*ast.NonNull); ok {
		if selection.All || len(selection.Values) == 0 {
			return nil, fmt.Errorf("%w: required GraphQL parameter has no value", ErrInvalid)
		}
		return graphQLSelection(required.Type, selection)
	}
	if selection.All {
		return nil, nil
	}
	if list, ok := typ.(*ast.List); ok {
		values := []any{}
		for _, value := range selection.Values {
			converted, err := graphQLSelection(list.Type, Selection{Values: []string{value}})
			if err != nil {
				return nil, err
			}
			values = append(values, converted)
		}
		return values, nil
	}
	if len(selection.Values) != 1 {
		return nil, fmt.Errorf("%w: GraphQL scalar requires one value", ErrInvalid)
	}
	named, ok := typ.(*ast.Named)
	if !ok {
		return nil, ErrInvalid
	}
	value := selection.Values[0]
	switch named.Name.Value {
	case "String", "ID":
		return value, nil
	case "Float":
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, ErrInvalid
		}
		return number, nil
	case "Int":
		number, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return nil, ErrInvalid
		}
		return int32(number), nil
	case "Boolean":
		if value != "true" && value != "false" {
			return nil, ErrInvalid
		}
		return value == "true", nil
	default:
		return nil, fmt.Errorf("%w: query variables bind to scalar GraphQL inputs", ErrInvalid)
	}
}
