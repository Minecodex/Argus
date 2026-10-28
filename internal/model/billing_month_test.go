package model

import (
	"testing"
	"time"
)

func TestBillingMonthUsesEnterpriseCivilMonth(t *testing.T) {
	for _, test := range []struct{ at, zone, want string }{
		{"2026-09-30T15:59:59Z", "Asia/Shanghai", "2026-09-01"}, {"2026-09-30T16:00:00Z", "Asia/Shanghai", "2026-10-01"},
		{"2026-11-01T03:59:59Z", "America/New_York", "2026-10-01"}, {"2026-11-01T04:00:00Z", "America/New_York", "2026-11-01"},
	} {
		at, _ := time.Parse(time.RFC3339, test.at)
		got, err := billingMonth(at, test.zone)
		if err != nil || got.Format("2006-01-02") != test.want {
			t.Fatalf("%+v: %v %v", test, got, err)
		}
	}
}
