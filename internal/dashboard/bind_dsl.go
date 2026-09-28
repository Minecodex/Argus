package dashboard

import (
	"maps"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func bindDSL(language queryengine.Language, query DSL, parameters map[string]Selection) (DSL, error) {
	if len(parameters) == 0 {
		return query, nil
	}
	switch language {
	case queryengine.LanguagePromQL:
		value, err := bindPromQL(query.Expression, parameters)
		query.Expression = value
		return query, err
	case queryengine.LanguageKQL:
		return bindKQL(query, parameters)
	case queryengine.LanguageTrace:
		types, err := graphQLParameterTypes(query)
		if err != nil {
			return query, err
		}
		query.Variables = maps.Clone(query.Variables)
		if query.Variables == nil {
			query.Variables = map[string]any{}
		}
		for name, selection := range parameters {
			typ, ok := types[name]
			if !ok {
				return query, ErrInvalid
			}
			value, e := graphQLSelection(typ, selection)
			if e != nil {
				return query, e
			}
			query.Variables[name] = value
		}
		return query, nil
	default:
		return query, ErrInvalid
	}
}
