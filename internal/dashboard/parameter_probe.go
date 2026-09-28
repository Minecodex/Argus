package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/graphql-go/graphql/language/ast"
	"slices"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

// CompileTarget validates the authored definition. Parameterized definitions
// have no executable query until execution resolves and freezes their inputs.
func CompileTarget(target Target) (CompiledTarget, error) {
	probe := target
	deferred := len(target.ParameterBindings) > 0
	if b := target.SourceDefinition.Builder; b != nil {
		builder := *b
		builder.Filters = slices.Clone(b.Filters)
		for i := range builder.Filters {
			f := &builder.Filters[i]
			if f.Variable == "" && f.LocalParameter == "" && f.DrilldownInput == "" {
				continue
			}
			deferred = true
			f.Value = "validation"
			if f.Field == "sourceId" || f.Field == "resourceId" {
				f.Value = uuid.Nil.String()
			}
			if numericFilter(*f) {
				f.Value = "0"
			}
			f.Variable, f.LocalParameter, f.Values = "", "", nil
			f.DrilldownInput = ""
		}
		probe.SourceDefinition.Builder = &builder
	} else if dsl := target.SourceDefinition.DSL; dsl != nil && deferred {
		if err := validateQueryReferences(target, *dsl); err != nil {
			return CompiledTarget{}, err
		}
		values := map[string]Selection{}
		for _, binding := range target.ParameterBindings {
			values[binding.Parameter] = Selection{Values: []string{"0"}}
		}
		if target.Language == queryengine.LanguageTrace {
			types, err := graphQLParameterTypes(*dsl)
			if err != nil {
				return CompiledTarget{}, err
			}
			for name := range values {
				t := types[name]
				if t == nil {
					return CompiledTarget{}, fmt.Errorf("undefined GraphQL parameter %s", name)
				}
				switch graphQLScalarName(t) {
				case "Boolean":
					values[name] = Selection{Values: []string{"false"}}
				case "String", "ID":
					values[name] = Selection{Values: []string{uuid.Nil.String()}}
				case "Int":
					values[name] = Selection{Values: []string{"1"}}
				}
			}
		}
		query, err := bindDSL(target.Language, *dsl, values)
		if err != nil {
			return CompiledTarget{}, err
		}
		probe.SourceDefinition.DSL = &query
	}
	probe.ParameterBindings = nil
	compiled, err := compileConcreteTarget(probe, !deferred)
	if err != nil || !deferred {
		return compiled, err
	}
	compiled.Deferred, compiled.Query = true, DSL{}
	encoded, _ := json.Marshal(struct {
		Target   Target
		Compiler string
	}{target, CompilerVersion})
	digest := sha256.Sum256(encoded)
	compiled.Hash = hex.EncodeToString(digest[:])
	return compiled, nil
}

func graphQLScalarName(t ast.Type) string {
	switch n := t.(type) {
	case *ast.NonNull:
		return graphQLScalarName(n.Type)
	case *ast.List:
		return graphQLScalarName(n.Type)
	case *ast.Named:
		return n.Name.Value
	}
	return ""
}

func numericFilter(f Filter) bool {
	return f.Field == "severity_number" || f.Field == "durationMin" || f.Field == "durationMax" || f.Operator == ">" || f.Operator == ">=" || f.Operator == "<" || f.Operator == "<="
}
