package skywalking

import (
	"errors"
	"testing"
)

func TestApplicationPortalQueriesUseActualReadOnlySchema(t *testing.T) {
	queries := ApplicationAPMQueries()
	if len(queries) != 7 {
		t.Fatal("application view registry drift")
	}
	for view, query := range queries {
		t.Run(view, func(t *testing.T) {
			for _, services := range []any{nil, []any{"d9320543-dd84-4f52-b1d4-60b61a54668e"}} {
				variables := map[string]any{"serviceIds": services}
				if view == "trace_detail" {
					variables = map[string]any{"traceId": "0123456789abcdef0123456789abcdef", "sourceId": "d9320543-dd84-4f52-b1d4-60b61a54668e", "resourceId": "d9320543-dd84-4f52-b1d4-60b61a54668e"}
				}
				if view == "red" {
					variables["bucketSeconds"] = 30
				}
				if err := ValidateInputs(query, "ApplicationAPM", variables); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestApplicationPortalQueryCanNarrowItsPageWithinAnExistingBudget(t *testing.T) {
	for _, limit := range []int{1, 10, 100} {
		query, err := ApplicationAPMQuery("services", limit)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateInputs(query, "ApplicationAPM", map[string]any{"serviceIds": nil}); err != nil {
			t.Fatal(err)
		}
		if limit == 100 && query != ApplicationAPMQueries()["services"] {
			t.Fatal("managed default query changed")
		}
	}
	for _, limit := range []int{-1, 0, 101} {
		if _, err := ApplicationAPMQuery("services", limit); !errors.Is(err, ErrBudget) {
			t.Fatal("invalid page limit accepted", limit, err)
		}
	}
	if _, err := ApplicationAPMQuery("unknown", 10); err == nil {
		t.Fatal("unknown view accepted")
	}
}
