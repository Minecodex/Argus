// Package sourcekey defines bounded source-version selectors. Namespace facets
// select rows inside a source; they never grant access to its whole Cluster.
package sourcekey

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func Parse(value string) (id uuid.UUID, revision int64, namespace string, ok bool) {
	base, namespace, facet := strings.Cut(value, "@")
	idText, revisionText, found := strings.Cut(base, ":")
	var e1, e2 error
	id, e1 = uuid.Parse(idText)
	revision, e2 = strconv.ParseInt(revisionText, 10, 64)
	if !found || e1 != nil || e2 != nil || id == uuid.Nil || revision < 1 || id.String() != idText || strconv.FormatInt(revision, 10) != revisionText {
		return id, revision, namespace, false
	}
	if facet {
		ns, err := uuid.Parse(namespace)
		if err != nil || ns == uuid.Nil || ns.String() != namespace {
			return id, revision, namespace, false
		}
	}
	return id, revision, namespace, true
}

// Predicate's column expressions are constants authored by query adapters.
// Every selected facet is applied before aggregation or catalogue discovery.
func Predicate(column, namespaceExpression string, keys []string) (string, []any) {
	plain, facets := []string{}, []string{}
	for _, key := range keys {
		if strings.Contains(key, "@") {
			facets = append(facets, key)
		} else {
			plain = append(plain, key)
		}
	}
	if len(facets) == 0 {
		return column + " IN (?)", []any{plain}
	}
	filter := "concat(" + column + ",'@'," + namespaceExpression + ") IN (?)"
	if len(plain) == 0 {
		return filter, []any{facets}
	}
	return "(" + column + " IN (?) OR " + filter + ")", []any{plain, facets}
}

// BaseKeys is only used after series IDs have already been filtered by the
// complete selectors. Sample rows have no resource-attribute map of their own.
func BaseKeys(keys []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range keys {
		base, _, _ := strings.Cut(value, "@")
		if !seen[base] {
			result = append(result, base)
			seen[base] = true
		}
	}
	return result
}
