package skywalking

import (
	"fmt"

	"github.com/graphql-go/graphql/language/ast"
)

type GraphIdentity struct{ TraceID, SourceID, ResourceID string }

// GraphIdentityFromQuery only accepts one unconditional anchored graph query.
// A dashboard expansion cannot smuggle a second, unrelated lookup in an alias.
func GraphIdentityFromQuery(document, operation string, variables map[string]any) (GraphIdentity, error) {
	var identity GraphIdentity
	d, err := parseResultDocument(document, operation)
	if err != nil {
		return identity, err
	}
	if len(d.roots) != 1 || d.roots[0].Name.Value != "queryTraceGraph" || len(d.roots[0].Directives) > 0 {
		return identity, fmt.Errorf("authorized expansion requires one anchored trace graph")
	}
	values := map[string]string{}
	for _, arg := range d.roots[0].Arguments {
		switch arg.Name.Value {
		case "traceId", "sourceId", "resourceId":
		default:
			continue
		}
		switch value := arg.Value.(type) {
		case *ast.StringValue:
			values[arg.Name.Value] = value.Value
		case *ast.Variable:
			values[arg.Name.Value], _ = variables[value.Name.Value].(string)
		default:
			return identity, fmt.Errorf("trace anchor must use scalar inputs")
		}
	}
	identity = GraphIdentity{values["traceId"], values["sourceId"], values["resourceId"]}
	if identity.TraceID == "" || !validUUID(identity.SourceID) || !validUUID(identity.ResourceID) {
		return identity, fmt.Errorf("trace anchor is incomplete")
	}
	return identity, nil
}
