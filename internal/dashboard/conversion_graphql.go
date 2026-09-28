package dashboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	gqlparser "github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/printer"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func graphDocument(expression string) (*ast.Document, *ast.OperationDefinition, *ast.Field, error) {
	doc, err := gqlparser.Parse(gqlparser.ParseParams{Source: expression})
	if err != nil {
		return nil, nil, nil, err
	}
	if len(doc.Definitions) != 1 {
		return nil, nil, nil, fmt.Errorf("multiple operations or fragments require statement mode")
	}
	op, ok := doc.Definitions[0].(*ast.OperationDefinition)
	if !ok || op.Operation != "query" || len(op.Directives) > 0 || op.SelectionSet == nil || len(op.SelectionSet.Selections) != 1 {
		return nil, nil, nil, fmt.Errorf("one read-only root is required")
	}
	root, ok := op.SelectionSet.Selections[0].(*ast.Field)
	if !ok {
		return nil, nil, nil, fmt.Errorf("root fragments require statement mode")
	}
	return doc, op, root, nil
}
func graphName(value string) *ast.Name { return ast.NewName(&ast.Name{Value: value}) }
func graphVariable(value string) *ast.Variable {
	return ast.NewVariable(&ast.Variable{Name: graphName(value)})
}
func graphNamed(value string) ast.Type { return ast.NewNamed(&ast.Named{Name: graphName(value)}) }
func graphListType() ast.Type {
	return ast.NewList(&ast.List{Type: ast.NewNonNull(&ast.NonNull{Type: graphNamed("String")})})
}
func graphFilterType(b Builder, f Filter) ast.Type {
	if apmAttributeField(f.Field) {
		return graphListType()
	}
	var typ ast.Type = graphNamed("String")
	if numericFilter(f) {
		typ = graphNamed("Float")
	}
	if b.Operation == "trace_graph" && (f.Field == "traceId" || f.Field == "sourceId" || f.Field == "resourceId") || b.Operation == "detail" && f.Field == "traceId" {
		typ = ast.NewNonNull(&ast.NonNull{Type: typ})
	}
	return typ
}
func traceBuilderTemplate(b Builder, names map[int]string) (DSL, error) {
	base := b
	base.Filters = nil
	for i, f := range b.Filters {
		if names[i] == "" {
			base.Filters = append(base.Filters, f)
		}
	}
	query, err := compileBuilder(queryengine.LanguageTrace, base)
	if err != nil {
		return query, err
	}
	if len(names) == 0 {
		return query, nil
	}
	doc, op, root, err := graphDocument(query.Expression)
	if err != nil {
		return query, err
	}
	var attributes []*ast.ObjectValue
	var existingAttributes *ast.Argument
	for _, arg := range root.Arguments {
		if arg.Name.Value == "filters" {
			existingAttributes = arg
			list := arg.Value.(*ast.ListValue)
			for _, value := range list.Values {
				attributes = append(attributes, value.(*ast.ObjectValue))
			}
		}
	}
	typed := map[string]string{}
	attributeIndex := 0
	outputAttributes := []ast.Value{}
	for i, f := range b.Filters {
		name := names[i]
		if name == "" {
			if apmAttributeField(f.Field) {
				outputAttributes = append(outputAttributes, attributes[attributeIndex])
				attributeIndex++
			}
			continue
		}
		var typ ast.Type = graphNamed("String")
		if apmAttributeField(f.Field) {
			if f.Operator != "=" && f.Operator != "!=" {
				return query, fmt.Errorf("attribute conversion requires set equality")
			}
			typ = graphListType()
			literal, err := compileAttributeInput(Filter{Field: f.Field, Operator: f.Operator, Value: ""})
			if err != nil {
				return query, err
			}
			_, _, temporary, err := graphDocument("query { queryAPMServices(filters:[" + literal + "]) {status} }")
			if err != nil {
				return query, err
			}
			object := temporary.Arguments[0].Value.(*ast.ListValue).Values[0].(*ast.ObjectValue)
			for _, field := range object.Fields {
				if field.Name.Value == "values" {
					field.Value = graphVariable(name)
				}
			}
			outputAttributes = append(outputAttributes, object)
		} else {
			if f.Operator != "=" {
				return query, fmt.Errorf("trace conversion requires scalar equality")
			}
			if numericFilter(f) {
				typ = graphNamed("Float")
			}
			if root.Name.Value == "queryTraceGraph" && (f.Field == "traceId" || f.Field == "sourceId" || f.Field == "resourceId") || root.Name.Value == "queryTrace" && f.Field == "traceId" {
				typ = ast.NewNonNull(&ast.NonNull{Type: typ})
			}
			root.Arguments = append(root.Arguments, ast.NewArgument(&ast.Argument{Name: graphName(f.Field), Value: graphVariable(name)}))
		}
		signature := fmt.Sprint(printer.Print(typ))
		if previous, ok := typed[name]; ok {
			if previous != signature {
				return query, fmt.Errorf("one parameter is used with incompatible GraphQL types")
			}
		} else {
			typed[name] = signature
			op.VariableDefinitions = append(op.VariableDefinitions, ast.NewVariableDefinition(&ast.VariableDefinition{Variable: graphVariable(name), Type: typ}))
		}
	}
	if len(outputAttributes) > 0 {
		value := ast.NewListValue(&ast.ListValue{Values: outputAttributes})
		if existingAttributes != nil {
			existingAttributes.Value = value
		} else {
			root.Arguments = append(root.Arguments, ast.NewArgument(&ast.Argument{Name: graphName("filters"), Value: value}))
		}
	}
	query.Expression = fmt.Sprint(printer.Print(doc))
	return query, nil
}

func traceBuilderFromDSL(target Target) (Builder, error) {
	b := Builder{Filters: []Filter{}, GroupBy: []string{}}
	query := target.SourceDefinition.DSL
	if query.Operation != "" || len(query.Variables) > 0 {
		return b, fmt.Errorf("operation selection and static variable objects require statement mode")
	}
	_, op, root, err := graphDocument(query.Expression)
	if err != nil {
		return b, err
	}
	if op.Name != nil || len(root.Directives) > 0 || root.Alias != nil {
		return b, fmt.Errorf("named operations, directives and aliases require statement mode")
	}
	b.Operation = map[string]string{"queryTraces": "list", "queryTrace": "detail", "queryTraceGraph": "trace_graph", "queryAPMServices": "apm_services", "queryAPMInstances": "apm_instances", "queryAPMEndpoints": "apm_endpoints", "queryAPMRED": "apm_red", "queryAPMTopology": "apm_topology"}[root.Name.Value]
	if b.Operation == "" {
		return b, fmt.Errorf("unsupported trace root")
	}
	for _, arg := range root.Arguments {
		name := arg.Name.Value
		switch name {
		case "pageSize", "limit", "bucketSeconds":
			value, ok := arg.Value.(*ast.IntValue)
			if !ok {
				return b, fmt.Errorf("limit and bucket require fixed integers")
			}
			number, e := strconv.Atoi(value.Value)
			if e != nil {
				return b, e
			}
			if name == "bucketSeconds" {
				b.BucketSeconds = number
			} else {
				b.Limit = number
			}
		case "filters":
			list, ok := arg.Value.(*ast.ListValue)
			if !ok {
				return b, fmt.Errorf("attribute filter structure requires statement mode")
			}
			for _, value := range list.Values {
				filter, e := graphAttributeFilter(value, target.ParameterBindings)
				if e != nil {
					return b, e
				}
				b.Filters = append(b.Filters, filter)
			}
		default:
			if !strings.Contains(" serviceName serviceInstanceName operationName status sourceId resourceId traceId durationMin durationMax ", " "+name+" ") {
				return b, fmt.Errorf("unsupported trace argument %s", name)
			}
			f := Filter{Field: name, Operator: "="}
			f, err = graphFilterValue(f, arg.Value, target.ParameterBindings)
			if err != nil {
				return b, err
			}
			b.Filters = append(b.Filters, f)
		}
	}
	return b, nil
}
func graphFilterValue(f Filter, value ast.Value, bindings []ParameterBinding) (Filter, error) {
	switch v := value.(type) {
	case *ast.Variable:
		return restoreFilterParameter(f, v.Name.Value, bindings)
	case *ast.StringValue:
		f.Value = v.Value
	case *ast.IntValue:
		f.Value = v.Value
	case *ast.FloatValue:
		f.Value = v.Value
	default:
		return f, fmt.Errorf("unsupported filter literal")
	}
	return f, nil
}
func graphAttributeFilter(value ast.Value, bindings []ParameterBinding) (Filter, error) {
	var f Filter
	object, ok := value.(*ast.ObjectValue)
	if !ok {
		return f, fmt.Errorf("attribute filter needs a fixed object")
	}
	fields := map[string]ast.Value{}
	for _, field := range object.Fields {
		if _, exists := fields[field.Name.Value]; exists {
			return f, fmt.Errorf("duplicate attribute field")
		}
		fields[field.Name.Value] = field.Value
	}
	key, ok := fields["key"].(*ast.StringValue)
	if !ok {
		return f, fmt.Errorf("attribute key must be fixed")
	}
	resource, ok := fields["resource"].(*ast.BooleanValue)
	if !ok {
		return f, fmt.Errorf("attribute source must be fixed")
	}
	f.Field = "attributes." + key.Value
	if resource.Value {
		f.Field = "resource_attributes." + key.Value
	}
	f.Operator = "="
	if negated, present := fields["negate"]; present {
		v, ok := negated.(*ast.BooleanValue)
		if !ok {
			return f, fmt.Errorf("attribute polarity must be fixed")
		}
		if v.Value {
			f.Operator = "!="
		}
	}
	if variable, ok := fields["values"].(*ast.Variable); ok {
		return restoreFilterParameter(f, variable.Name.Value, bindings)
	}
	list, ok := fields["values"].(*ast.ListValue)
	if !ok {
		return f, fmt.Errorf("attribute values must be a list or parameter")
	}
	for _, value := range list.Values {
		v, ok := value.(*ast.StringValue)
		if !ok {
			return f, fmt.Errorf("attribute values must be strings")
		}
		f.Values = append(f.Values, v.Value)
	}
	if len(f.Values) == 0 {
		return f, fmt.Errorf("empty attribute value set")
	}
	return f, nil
}
