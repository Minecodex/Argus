package telemetry

import (
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"reflect"
	"strings"
	"testing"
)

func TestObservationCompletionPredicatesKeepNativeSemantics(t *testing.T) {
	for _, op := range []string{"=~", "!~"} {
		sql, args, err := catalogFilterSQL("metrics", CatalogFilter{Field: "service", Operator: op, Values: []string{"api.*"}})
		if err != nil || !strings.Contains(sql, "match(toString(labels[?]), ?)") || !reflect.DeepEqual(args, []any{"service", "^(?s:api.*)$"}) {
			t.Fatalf("bad regex narrowing: %s %v %v", sql, args, err)
		}
		if strings.HasPrefix(sql, "NOT ") != (op == "!~") {
			t.Fatal("negation changed")
		}
	}
	if _, _, err := catalogFilterSQL("metrics", CatalogFilter{Field: "service", Operator: "=~", Values: []string{"["}}); err == nil {
		t.Fatal("invalid regexp accepted")
	}
	for _, value := range []string{"api", "api*", "a?b"} {
		actual, args, err := catalogFilterSQL("logs", CatalogFilter{Field: "service_name", Operator: ":", Values: []string{value}})
		compiler := kql.Compiler{}
		expected, e := compiler.Compile(kql.Predicate{Field: "service_name", Op: kql.OpContains, Value: value, Quoted: true})
		if err != nil || e != nil || actual != expected || !reflect.DeepEqual(args, compiler.Args) {
			t.Fatal("KQL completion changed contains/wildcard semantics")
		}
	}
	if ValidateCatalogFilters("traces", []CatalogFilter{{Field: "service_name", Operator: "=~", Values: []string{".*"}}}) == nil {
		t.Fatal("PromQL-only regexp accepted for another language")
	}
}
