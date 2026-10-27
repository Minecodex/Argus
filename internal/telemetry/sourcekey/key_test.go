package sourcekey

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNamespaceSelectorsAreCanonicalAndNeverBecomeWholeSource(t *testing.T) {
	base := uuid.NewString() + ":42"
	namespace := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	id, revision, ns, ok := Parse(base + "@" + namespace)
	if !ok || id == uuid.Nil || revision != 42 || ns != namespace {
		t.Fatal("valid Namespace selector rejected")
	}
	for _, value := range []string{base + "@", base + "@" + namespace + "@" + namespace, base + "@../orders", base + "@" + strings.ToUpper(namespace), strings.Replace(base, ":42", ":042", 1), uuid.Nil.String() + ":1"} {
		if _, _, _, ok := Parse(value); ok {
			t.Fatal("non-canonical scope accepted", value)
		}
	}
	predicate, args := Predicate("source_key", "resource_attributes['argus.namespace.uid']", []string{base + "@" + namespace})
	if strings.Contains(predicate, " OR ") || !strings.Contains(predicate, "argus.namespace.uid") || !slices.Equal(args[0].([]string), []string{base + "@" + namespace}) {
		t.Fatal("Namespace-only selection acquired a full-source alternative")
	}
	predicate, args = Predicate("source_key", "resource_attributes['argus.namespace.uid']", []string{base, base + "@" + namespace})
	if !strings.Contains(predicate, " OR ") || len(args) != 2 {
		t.Fatal("mixed explicitly authorized scope lost its union")
	}
}
