package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type contextProviderFixture struct{ facts map[string]any }

func (p contextProviderFixture) BuildTools(context.Context, Principal) (Contribution, error) {
	return Contribution{}, nil
}
func (p contextProviderFixture) ContextFacts(context.Context, ContextRequest) (map[string]any, error) {
	return p.facts, nil
}

func TestReferenceIndexKeepsExactRecordsAndExplicitBudgets(t *testing.T) {
	path := "/workspace/结果 741dc561-54a5-4550-b329-52f396548fe1/f953b1e5-fb8d-4deb-9f3f-512191a6d3e5.json"
	items := []map[string]any{{"path": path, "hash": strings.Repeat("a", 64)}, {"path": strings.Repeat("x", 8192)}}
	first := BoundReferenceIndex(items, true, 64, 1024)
	if first["complete"] != false || len(first["items"].([]map[string]any)) != 1 || first["items"].([]map[string]any)[0]["path"] != path {
		t.Fatal("reference was truncated or incompleteness lost", first)
	}
	encoded, _ := json.Marshal(first)
	if len(encoded) > 1024 {
		t.Fatal("reference budget exceeded")
	}
	for i := 0; i < 10; i++ {
		next := BoundReferenceIndex(items, true, 64, 1024)
		if next["index_hash"] != first["index_hash"] {
			t.Fatal("unchanged reference identity drifted")
		}
	}
}

func TestContextProvidersValidateAndRejectNamespaceCollisions(t *testing.T) {
	denied := errors.New("revoked")
	factory := CompositeFactory{Providers: []Provider{contextProviderFixture{map[string]any{"files": "a"}}}, Validate: func(context.Context, Principal) error { return denied }}
	if _, err := factory.ContextFacts(t.Context(), ContextRequest{}); !errors.Is(err, denied) {
		t.Fatal("provider bypassed current authorization", err)
	}
	factory.Validate = nil
	factory.Providers = append(factory.Providers, contextProviderFixture{map[string]any{"files": "b"}})
	if _, err := factory.ContextFacts(t.Context(), ContextRequest{}); err == nil {
		t.Fatal("duplicate provider silently overwrote context")
	}
}
