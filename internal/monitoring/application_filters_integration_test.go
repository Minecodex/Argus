package monitoring

import (
	"errors"
	"github.com/kakj-go/Argus/internal/pagination"
	"testing"
)

func TestPostgresApplicationEnvironmentBeforePagination(t *testing.T) {
	f := newFixture(t)
	f.application("Orders production one", "production")
	f.application("Orders production two", "production")
	f.application("Orders testing", "testing")
	f.application("Other production", "production")
	input := ListInput{Environment: "production", Search: " orders ", Limit: 1}
	first, err := f.s.Applications(t.Context(), f.a, input)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first: %+v %v", first, err)
	}
	input.Cursor = first.NextCursor
	second, err := f.s.Applications(t.Context(), f.a, input)
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" || first.Items[0].ID == second.Items[0].ID {
		t.Fatalf("second: %+v %v", second, err)
	}
	for _, item := range append(first.Items, second.Items...) {
		if item.Environment != "production" {
			t.Fatal(item)
		}
	}
	input.Environment = "testing"
	if _, err = f.s.Applications(t.Context(), f.a, input); !errors.Is(err, pagination.ErrInvalid) {
		t.Fatalf("changed filter cursor accepted: %v", err)
	}
	input.Cursor = ""
	filtered, err := f.s.Applications(t.Context(), f.a, input)
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Name != "Orders testing" {
		t.Fatalf("filtered: %+v %v", filtered, err)
	}
	hidden, err := f.s.Applications(t.Context(), f.other, input)
	if err != nil || len(hidden.Items) != 0 {
		t.Fatalf("authorization widened: %+v %v", hidden, err)
	}
	input.Environment = "invalid environment"
	if _, err = f.s.Applications(t.Context(), f.a, input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid environment accepted: %v", err)
	}
}
