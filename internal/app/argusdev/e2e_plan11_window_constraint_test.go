package argusdev

import (
	"errors"
	"testing"
	"time"
)

func TestComparisonSeparatesNarrowTaskFailureFromActualScopeExpansion(t *testing.T) {
	w := plan11ComparisonWindow{From: time.Date(2026, 10, 31, 8, 34, 44, 0, time.UTC), To: time.Date(2026, 10, 31, 8, 39, 44, 0, time.UTC)}
	args := func(from, to, host string) map[string]any {
		return map[string]any{"from": from, "to": to, "resource_ids": []any{host}}
	}
	for _, tc := range []struct {
		name, from, to, host string
		failed, hard         bool
	}{
		{"exact", "2026-10-31T08:34:44Z", "2026-10-31T08:39:44Z", "host", false, false},
		{"actual-r27-shortened-end", "2026-10-31T08:34:44Z", "2026-10-31T08:35:44Z", "host", true, false},
		{"shortened-start", "2026-10-31T08:38:00Z", "2026-10-31T08:39:44Z", "host", true, false},
		{"outside-start", "2026-10-31T08:34:43Z", "2026-10-31T08:39:44Z", "host", true, true},
		{"outside-end", "2026-10-31T08:34:44Z", "2026-10-31T08:39:45Z", "host", true, true},
		{"other-host", "2026-10-31T08:34:44Z", "2026-10-31T08:39:44Z", "other", true, true},
		{"invalid-time", "bad", "2026-10-31T08:39:44Z", "host", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := plan11ComparisonQueryConstraint(args(tc.from, tc.to, tc.host), "host", w)
			if (err != nil) != tc.failed || errors.Is(err, errPlan11ComparisonBoundary) != tc.hard {
				t.Fatal("constraint classification changed the fixed-window or safety gate", err)
			}
		})
	}
}
