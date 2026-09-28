package skywalking

import (
	"github.com/graphql-go/graphql/language/ast"
)

func HasTraceIdentity(document, operation string) bool {
	d, err := parseResultDocument(document, operation)
	if err != nil || len(d.roots) != 1 {
		return false
	}
	root := d.roots[0]
	var fields *ast.SelectionSet
	switch root.Name.Value {
	case "queryTrace":
		fields = root.SelectionSet
	case "queryTraceGraph":
		if row := d.requiredField(root.SelectionSet, "spans"); row != nil {
			fields = row.SelectionSet
		}
	case "queryTraces", "queryBasicTraces", "queryBasicTracesByName":
		if row := d.requiredField(root.SelectionSet, "traces"); row != nil {
			fields = row.SelectionSet
		}
	}
	for _, name := range []string{"traceId", "sourceId", "resourceId"} {
		if d.requiredField(fields, name) == nil {
			return false
		}
	}
	return true
}
