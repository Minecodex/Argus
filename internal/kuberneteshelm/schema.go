package kuberneteshelm

import (
	"bytes"
	"encoding/json"

	"github.com/santhosh-tekuri/jsonschema/v6"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

type offlineSchemaLoader struct{}

func (offlineSchemaLoader) Load(string) (any, error) { return nil, ErrDenied }

// Helm's default schema loader supports file/http URLs. Keep validation while
// replacing only that loader; embedded definitions and standard drafts work.
func validateOfflineSchemas(ch *chart.Chart, values any) error {
	raw, e := json.Marshal(values)
	if e != nil {
		return ErrInvalid
	}
	defer clear(raw)
	value, e := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if e != nil {
		return ErrInvalid
	}
	return validateChartSchema(ch, value)
}
func validateChartSchema(ch *chart.Chart, values any) error {
	if len(ch.Schema) > MaxValuesBytes {
		return ErrBudget
	}
	if len(ch.Schema) > 0 {
		schema, e := jsonschema.UnmarshalJSON(bytes.NewReader(ch.Schema))
		if e != nil {
			return ErrInvalid
		}
		compiler := jsonschema.NewCompiler()
		compiler.UseLoader(offlineSchemaLoader{})
		if e = compiler.AddResource("urn:argus:helm-values", schema); e != nil {
			return ErrInvalid
		}
		validator, e := compiler.Compile("urn:argus:helm-values")
		if e != nil {
			return ErrInvalid
		}
		if e = validator.Validate(values); e != nil {
			return ErrInvalid
		}
	}
	value, _ := values.(map[string]any)
	for _, dependency := range ch.Dependencies() {
		if v, ok := value[dependency.Name()]; ok {
			if e := validateChartSchema(dependency, v); e != nil {
				return e
			}
		}
	}
	return nil
}
