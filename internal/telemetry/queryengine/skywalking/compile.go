package skywalking

import (
	"context"
	"fmt"
	graphqlparser "github.com/graphql-go/graphql/language/parser"
)

type inputValidationKey struct{}

func ValidateInputs(document, operation string, variables map[string]any) error {
	if err := validateQuery(document, variables); err != nil {
		return err
	}
	response := traceSchema.Exec(context.WithValue(context.Background(), inputValidationKey{}, true), document, operation, variables)
	if len(response.Errors) > 0 {
		return fmt.Errorf("GraphQL inputs: %s", response.Errors[0].Message)
	}
	return nil
}

// Validate checks the exact read-only schema and complexity rules used by the
// engine without accessing storage or confusing no_data with invalid syntax.
func Validate(document string) error {
	return validateQuery(document, nil)
}

func validateQuery(document string, variables map[string]any) error {
	parsed, err := graphqlparser.Parse(graphqlparser.ParseParams{Source: document})
	if err != nil {
		return err
	}
	depth, fields, err := validateDocument(parsed)
	if err != nil {
		return err
	}
	if depth > 8 || fields > 100 {
		return fmt.Errorf("GraphQL complexity budget exceeded")
	}
	if errs := traceSchema.ValidateWithVariables(document, variables); len(errs) > 0 {
		return fmt.Errorf("GraphQL schema: %s", errs[0].Message)
	}
	return nil
}
