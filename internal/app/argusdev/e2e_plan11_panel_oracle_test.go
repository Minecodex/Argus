package argusdev

import (
	"testing"
)

func TestPanelClaimUsesOriginalEpochAndPublishedOutputUnit(t *testing.T) {
	body := []byte(`[{"metric":{"state":"used"},"values":[[1793447566,"75.823569709"],[1793447656,"74.459892779"]]}]`)
	count, low, high := 2, 74.459892779, 75.823569709
	valid := plan11PanelClaim{PanelID: "memory", TargetID: "main", Hash: "sealed", Unit: "%", Count: &count, First: "2026-10-31T11:52:46Z", Last: "2026-10-31T11:54:16Z", Min: &low, Max: &high}
	if err := plan11MatchPanelClaim(valid, "%", "sealed", body); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*plan11PanelClaim){
		func(c *plan11PanelClaim) { c.First = "2026-10-31T11:52:16Z" },
		func(c *plan11PanelClaim) { c.First = "2026-10-31T11:49:26Z" },
		func(c *plan11PanelClaim) { c.Last = "2026-10-31T11:52:56Z" },
		func(c *plan11PanelClaim) { c.Unit = "1" },
		func(c *plan11PanelClaim) { value := low * 100; c.Min = &value },
		func(c *plan11PanelClaim) { c.Hash = "changed" },
		func(c *plan11PanelClaim) { c.Count = nil },
	} {
		changed := valid
		change(&changed)
		if err := plan11MatchPanelClaim(changed, "%", "sealed", body); err == nil {
			t.Fatal("wrong panel facts accepted", changed)
		}
	}
}

func TestDashboardNarrativeCannotContradictVerifiedClockFacts(t *testing.T) {
	allowed := map[string]bool{"11:49:16": true, "11:52:46": true, "11:53:16": true, "11:54:16": true}
	if err := plan11MatchEvidenceClockClaims("memory 11:52:46–11:54:16 UTC; fixed window starts 11:49:16", allowed); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"memory 11:52:16–11:54:16", "cpu 11:49:56–11:53:56", "memory 11:49:26–11:52:56"} {
		if err := plan11MatchEvidenceClockClaims(text, allowed); err == nil {
			t.Fatal("r30 prose error accepted", text)
		}
	}
}
