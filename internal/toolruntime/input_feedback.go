package toolruntime

import (
	"errors"
	"strings"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Report constraints and bounded field paths, never the rejected values or
// the validator's error string (which can include credentials and payloads).
func InputValidationError(cause error, prefix string) Error {
	issues := []map[string]any{}
	var validation *jsonschema.ValidationError
	var visit func(*jsonschema.ValidationError)
	visit = func(v *jsonschema.ValidationError) {
		if v == nil || len(issues) >= 16 {
			return
		}
		if len(v.Causes) > 0 {
			for _, child := range v.Causes {
				visit(child)
			}
			return
		}
		path := prefix
		for _, segment := range v.InstanceLocation[:min(len(v.InstanceLocation), 16)] {
			path += "/" + schemaFieldSegment(segment)
		}
		switch problem := v.ErrorKind.(type) {
		case *kind.Required:
			for _, field := range problem.Missing[:min(len(problem.Missing), 16-len(issues))] {
				issues = append(issues, map[string]any{"path": path + "/" + schemaFieldSegment(field), "constraint": "required"})
			}
		case *kind.Type:
			issues = append(issues, map[string]any{"path": path, "constraint": "type", "expected": problem.Want})
		case *kind.AdditionalProperties:
			for _, field := range problem.Properties[:min(len(problem.Properties), 16-len(issues))] {
				issues = append(issues, map[string]any{"path": path + "/" + schemaFieldSegment(field), "constraint": "additionalProperties", "message": "This field is not accepted."})
			}
		default:
			keyword := "schema"
			if v.ErrorKind != nil {
				if p := v.ErrorKind.KeywordPath(); len(p) > 0 {
					keyword = schemaFieldSegment(p[0])
				}
			}
			issues = append(issues, map[string]any{"path": path, "constraint": keyword})
		}
	}
	if errors.As(cause, &validation) {
		visit(validation)
	}
	return Error{Kind: "TOOL_INPUT_INVALID", Message: "Arguments do not match the tool schema. Fix the indicated fields and retry; no tool operation was executed.", Details: map[string]any{"schema_validation": true, "issues": issues, "more_issues_possible": len(issues) >= 16}}
}

func schemaFieldSegment(value string) string {
	if len(value) > 96 || strings.ContainsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.", r)
	}) {
		return "*"
	}
	return value
}
