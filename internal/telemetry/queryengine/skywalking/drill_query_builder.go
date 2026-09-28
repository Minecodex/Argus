package skywalking

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	graphqlparser "github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/printer"
)

type DerivedQuery struct {
	Expression    string
	UsedVariables []string
	RowVariables  map[string]string
}

// DeriveDrillQuery retains all business filters that are not replaced by a
// selected entity's stronger identity. It never edits the published display.
func DeriveDrillQuery(document, operation, root, projection string, rowArguments map[string]string) (DerivedQuery, error) {
	var result DerivedQuery
	d, err := parseResultDocument(document, operation)
	if err != nil {
		return result, err
	}
	if len(d.roots) != 1 || len(d.roots[0].Directives) > 0 {
		return result, fmt.Errorf("standard derivation needs one unconditional query")
	}
	parsed, err := graphqlparser.Parse(graphqlparser.ParseParams{Source: document})
	if err != nil {
		return result, err
	}
	definitions := map[string]*ast.VariableDefinition{}
	for _, node := range parsed.Definitions {
		op, ok := node.(*ast.OperationDefinition)
		if !ok || operation != "" && (op.Name == nil || op.Name.Value != operation) {
			continue
		}
		for _, v := range op.VariableDefinitions {
			definitions[v.Variable.Name.Value] = v
		}
	}
	used := map[string]bool{}
	var walk func(ast.Value)
	walk = func(value ast.Value) {
		switch node := value.(type) {
		case *ast.Variable:
			used[node.Name.Value] = true
		case *ast.ListValue:
			for _, v := range node.Values {
				walk(v)
			}
		case *ast.ObjectValue:
			for _, f := range node.Fields {
				walk(f.Value)
			}
		}
	}
	arguments := []string{}
	for _, arg := range d.roots[0].Arguments {
		if _, override := rowArguments[arg.Name.Value]; override {
			continue
		}
		keep := arg.Name.Value == "filters" || root == "queryTraces" && slices.Contains([]string{"serviceName", "serviceInstanceName", "operationName", "sourceId", "resourceId", "status", "durationMin", "durationMax", "tags"}, arg.Name.Value)
		if keep {
			arguments = append(arguments, fmt.Sprint(printer.Print(arg)))
			walk(arg.Value)
		}
	}
	declarations := []string{}
	for name := range used {
		result.UsedVariables = append(result.UsedVariables, name)
	}
	sort.Strings(result.UsedVariables)
	for _, name := range result.UsedVariables {
		if definitions[name] == nil {
			return result, fmt.Errorf("undefined variable")
		}
		declarations = append(declarations, fmt.Sprint(printer.Print(definitions[name])))
	}
	result.RowVariables = map[string]string{}
	fields := []string{}
	for field := range rowArguments {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		name := "argus_row_" + field
		for definitions[name] != nil || used[name] {
			name += "_"
		}
		used[name] = true
		result.RowVariables[rowArguments[field]] = name
		declarations = append(declarations, "$"+name+":String!")
		arguments = append(arguments, field+":$"+name)
	}
	if root == "queryTraces" {
		arguments = append(arguments, "pageSize:100")
	}
	decls := ""
	if len(declarations) > 0 {
		decls = "(" + strings.Join(declarations, ",") + ")"
	}
	args := ""
	if len(arguments) > 0 {
		args = "(" + strings.Join(arguments, ",") + ")"
	}
	result.Expression = "query" + decls + " {" + root + args + " {" + projection + "}}"
	return result, nil
}
