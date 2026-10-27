package skywalking

import (
	"errors"

	graphqlparser "github.com/graphql-go/graphql/language/parser"
)

// ValidateToolDocument runs only parser/schema checks. It never invokes a
// resolver or returns parser text that could echo arbitrary input values.
func ValidateToolDocument(document, operation string, variables map[string]any) error {
	invalid := errors.New("unsupported Argus Trace/APM document")
	if len(document) == 0 || len(document) > 65536 {
		return invalid
	}
	parsed, err := graphqlparser.Parse(graphqlparser.ParseParams{Source: document})
	if err != nil {
		return invalid
	}
	depth, fields, err := validateDocument(parsed)
	if err != nil || depth > 8 || fields > 100 {
		return invalid
	}
	if len(traceSchema.ValidateWithVariables(document, variables)) != 0 {
		return invalid
	}
	if _, err = ResultKind(document, operation); err != nil {
		return invalid
	}
	return nil
}
