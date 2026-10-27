package resourceref

import (
	"github.com/google/uuid"
	"testing"
)

func TestTypedScopeDoesNotRelabelOrExpandAnEmptySelection(t *testing.T) {
	id := uuid.New()
	if _, err := Normalize([]Ref{{Type: "host", ID: id}, {Type: "application", ID: id}}); err == nil {
		t.Fatal("ambiguous cross-type UUID allowed")
	}
	refs := []Ref{{Type: "application", ID: id}}
	if selected, err := Subset(refs, []uuid.UUID{}); err != nil || len(selected) != 0 {
		t.Fatal("empty selection expanded")
	}
	if _, err := Subset(refs, []uuid.UUID{uuid.New()}); err == nil {
		t.Fatal("unknown requested root silently dropped")
	}
	if _, err := Normalize([]Ref{{Type: "service", ID: id}}); err == nil {
		t.Fatal("service promoted to an independent grant root")
	}
}
