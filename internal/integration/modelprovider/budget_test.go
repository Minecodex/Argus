package modelprovider

import "testing"

func TestContextBudgetUsesTheSameExclusiveAdmissionBoundary(t *testing.T) {
	for _, tc := range []struct{ window, output, usable, hard int }{
		{32768, 1024, 27648, 23500}, {131072, 4096, 120423, 102359}, {4096, 1024, 0, 0},
	} {
		budget := NewContextBudget(tc.window, tc.output)
		if budget.Usable != tc.usable || budget.HardLimit != tc.hard {
			t.Fatalf("budget=%+v", budget)
		}
		if budget.Fits(tc.hard) || budget.Fits(tc.hard+1) {
			t.Fatal("hard boundary admitted")
		}
		if tc.hard > 0 && !budget.Fits(tc.hard-1) {
			t.Fatal("input below boundary rejected")
		}
	}
}
