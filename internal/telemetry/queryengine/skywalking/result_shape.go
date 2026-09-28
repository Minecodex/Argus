package skywalking

import (
	"fmt"
	"strings"

	"github.com/graphql-go/graphql/language/ast"
	graphqlparser "github.com/graphql-go/graphql/language/parser"
)

var apmShapes = map[string]string{"queryAPMServices": "apm_services", "queryAPMInstances": "apm_instances", "queryAPMEndpoints": "apm_endpoints", "queryAPMRED": "apm_red", "queryAPMTopology": "apm_topology"}

type resultDocument struct {
	roots     []*ast.Field
	fragments map[string]*ast.FragmentDefinition
}

func parseResultDocument(document, operation string) (resultDocument, error) {
	parsed, err := graphqlparser.Parse(graphqlparser.ParseParams{Source: document})
	if err != nil {
		return resultDocument{}, err
	}
	if _, _, err := validateDocument(parsed); err != nil {
		return resultDocument{}, err
	}
	result := resultDocument{fragments: map[string]*ast.FragmentDefinition{}}
	var selected *ast.OperationDefinition
	count := 0
	for _, definition := range parsed.Definitions {
		switch node := definition.(type) {
		case *ast.FragmentDefinition:
			result.fragments[node.Name.Value] = node
		case *ast.OperationDefinition:
			if operation != "" && (node.Name == nil || node.Name.Value != operation) {
				continue
			}
			selected = node
			count++
		}
	}
	if count != 1 {
		return result, fmt.Errorf("choose one GraphQL operation")
	}
	result.roots = result.fields(selected.SelectionSet)
	return result, nil
}

func (d resultDocument) fields(set *ast.SelectionSet) []*ast.Field {
	return d.fieldsWithConditions(set, nil)
}

func (d resultDocument) fieldsWithConditions(set *ast.SelectionSet, inherited []*ast.Directive) []*ast.Field {
	fields := []*ast.Field{}
	if set == nil {
		return fields
	}
	for _, selection := range set.Selections {
		switch node := selection.(type) {
		case *ast.Field:
			copy := *node
			copy.Directives = append(append([]*ast.Directive{}, inherited...), node.Directives...)
			fields = append(fields, &copy)
		case *ast.InlineFragment:
			conditions := append(append([]*ast.Directive{}, inherited...), node.Directives...)
			fields = append(fields, d.fieldsWithConditions(node.SelectionSet, conditions)...)
		case *ast.FragmentSpread:
			if f := d.fragments[node.Name.Value]; f != nil {
				conditions := append(append([]*ast.Directive{}, inherited...), node.Directives...)
				fields = append(fields, d.fieldsWithConditions(f.SelectionSet, conditions)...)
			}
		}
	}
	return fields
}

func ResultKind(document, operation string) (string, error) {
	d, err := parseResultDocument(document, operation)
	if err != nil {
		return "", err
	}
	kind := ""
	for _, root := range d.roots {
		shape := apmShapes[root.Name.Value]
		if root.Name.Value == "queryTraceGraph" {
			shape = "trace_graph"
		}
		if shape == "" {
			shape = "traces"
		}
		if kind != "" && kind != shape {
			return "trace_composite", nil
		}
		kind = shape
	}
	if kind == "" {
		return "traces", nil
	}
	return kind, nil
}

// Dashboard charts need stable projection fields even when the author uses a
// native query or fragments. Catalog/tool queries may select a smaller shape.
func ValidateAPMProjection(document, operation, kind string) error {
	if !strings.HasPrefix(kind, "apm_") {
		return nil
	}
	d, err := parseResultDocument(document, operation)
	if err != nil {
		return err
	}
	for _, root := range d.roots {
		if len(root.Directives) > 0 {
			return fmt.Errorf("APM chart root cannot be conditional")
		}
		rootFields := []string{"basis", "percentileMethod", "status", "coverage"}
		if kind == "apm_topology" {
			rootFields = append(rootFields, "nodes", "edges")
		} else {
			rootFields = append(rootFields, "rows")
		}
		for _, name := range rootFields {
			if d.requiredField(root.SelectionSet, name) == nil {
				return fmt.Errorf("APM chart requires %s", name)
			}
		}
		coverageFields := []string{"observedSpanCount", "requestSampleCount", "missingServiceCount", "missingInstanceCount", "missingOperationCount", "unknownSourceCount"}
		if kind == "apm_topology" {
			coverageFields = []string{"observedSpanCount", "missingParentCount", "ambiguousParentCount", "cyclicSpanCount", "missingServiceCount", "unknownSourceCount"}
		}
		for _, name := range coverageFields {
			if d.requiredField(d.requiredField(root.SelectionSet, "coverage").SelectionSet, name) == nil {
				return fmt.Errorf("APM chart requires coverage.%s", name)
			}
		}
		if kind == "apm_topology" {
			for field, names := range map[string][]string{"nodes": {"id", "sourceId", "resourceId", "serviceName"}, "edges": {"sourceNodeId", "targetNodeId", "sampleCount", "errorCount", "errorRate", "durationP95Ms"}} {
				for _, name := range names {
					if d.requiredField(d.requiredField(root.SelectionSet, field).SelectionSet, name) == nil {
						return fmt.Errorf("APM topology requires %s.%s", field, name)
					}
				}
			}
			continue
		}
		rows := d.requiredField(root.SelectionSet, "rows")
		names := []string{"sourceId", "resourceId", "serviceName", "sampleCount", "errorCount", "errorRate", "samplesPerSecond", "durationP95Ms"}
		switch kind {
		case "apm_instances":
			names = append(names, "instanceId")
		case "apm_endpoints":
			names = append(names, "operationName")
		case "apm_red":
			names = append(names, "timestamp", "intervalSeconds")
		}
		for _, name := range names {
			if d.requiredField(rows.SelectionSet, name) == nil {
				return fmt.Errorf("APM chart requires rows.%s", name)
			}
		}
	}
	return nil
}

func (d resultDocument) requiredField(set *ast.SelectionSet, name string) *ast.Field {
	for _, f := range d.fields(set) {
		if f.Name.Value == name && (f.Alias == nil || f.Alias.Value == name) && len(f.Directives) == 0 {
			return f
		}
	}
	return nil
}
