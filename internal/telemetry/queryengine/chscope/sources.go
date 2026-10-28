// Package chscope transports authorized source selectors as binary data rather
// than expanding an allowed large scope into ClickHouse SQL text.
package chscope

import (
	"context"
	"strings"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
)

const sourceTable = "argus_query_sources"

// WithSources preserves existing settings/progress and appends one immutable,
// per-query external table. An empty table matches no source.
func WithSources(ctx context.Context, keys []string) (context.Context, error) {
	table, err := ext.NewTable(sourceTable, ext.Column("selected_key", "String"), ext.Column("base_key", "String"), ext.Column("is_facet", "UInt8"))
	if err != nil {
		return ctx, err
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		base, _, facet := strings.Cut(key, "@")
		flag := uint8(0)
		if facet {
			flag = 1
		}
		if err = table.Append(key, base, flag); err != nil {
			return ctx, err
		}
	}
	return clickhouse.Context(ctx, clickhouse.WithExternalTable(table)), nil
}

// Columns are constant expressions authored by query adapters, never user SQL.
func ExactPredicate(column string) string {
	return column + " IN (SELECT selected_key FROM " + sourceTable + ")"
}

func BasePredicate(column string) string {
	return column + " IN (SELECT base_key FROM " + sourceTable + ")"
}

func FacetedPredicate(column, namespaceExpression string) string {
	return "(" + column + " IN (SELECT selected_key FROM " + sourceTable + " WHERE is_facet=0) OR concat(" + column + ",'@'," + namespaceExpression + ") IN (SELECT selected_key FROM " + sourceTable + " WHERE is_facet=1))"
}

// SelectedPredicate avoids reading the namespace metadata map when the frozen
// scope contains only whole sources. Mixed or namespace-only scopes retain the
// exact facet predicate; source selectors still travel in the external table.
func SelectedPredicate(column, namespaceExpression string, keys []string) string {
	for _, key := range keys {
		if strings.Contains(key, "@") {
			return FacetedPredicate(column, namespaceExpression)
		}
	}
	return ExactPredicate(column)
}
