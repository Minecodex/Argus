package telemetry

import "encoding/json"

// Native Trace tools carry the same parameterized document contract as the
// portal. Variables narrow data inside the already-resolved authorization scope.
func traceToolParameters(input map[string]any) (string, map[string]any, error) {
	operation := ""
	if value, present := input["operation_name"]; present {
		var ok bool
		operation, ok = value.(string)
		if !ok || len(operation) > 256 {
			return "", nil, ErrQueryInvalid
		}
	}
	var variables map[string]any
	if value := input["variables"]; value != nil {
		var ok bool
		variables, ok = value.(map[string]any)
		if !ok || len(variables) > 64 {
			return "", nil, ErrQueryInvalid
		}
		raw, err := json.Marshal(variables)
		if err != nil || len(raw) > 65536 {
			return "", nil, ErrQueryInvalid
		}
	}
	return operation, variables, nil
}
