package argusdev

import (
	"testing"
	"time"
)

func TestEvidenceClockEmptyPrefixIsBoundedAndOccurrenceSpecific(t *testing.T) {
	parse := func(value string) time.Time {
		at, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	prefix := plan11ComparisonWindow{From: parse("2026-10-31T16:44:31Z"), To: parse("2026-10-31T16:48:31Z")}
	allowed := map[string]bool{"16:44:31": true, "16:48:31": true, "16:49:31": true}
	for _, text := range []string{
		"16:44:31Z–16:48:01Z 区间未返回面板/网络评估点，仅表示返回时间分布。",
		"No returned evaluation points between 16:44:31Z and 16:48:01Z.",
	} {
		if err := plan11MatchEvidenceClockClaims(text, allowed, prefix); err != nil {
			t.Fatal("valid empty returned-evaluation prefix rejected", err)
		}
		if err := plan11MatchEvidenceClockClaims(text, allowed); err == nil {
			t.Fatal("unproved prefix received a clock exception")
		}
	}
	for _, text := range []string{
		"首点为 16:48:01Z，末点为 16:49:31Z。",
		"16:44:01Z–16:48:01Z 区间未返回评估点。",
		"16:44:31Z–16:48:32Z 区间未返回评估点。",
		"16:44:31Z–16:48:01Z 区间没有原始采样。",
		"16:44:31Z–16:48:01Z 区间未返回评估点。\n首点为 16:48:01Z。",
	} {
		if err := plan11MatchEvidenceClockClaims(text, allowed, prefix); err == nil {
			t.Fatal("wrong point, raw-coverage claim or unrelated occurrence was exempted", text)
		}
	}
	crossing := plan11ComparisonWindow{From: parse("2026-10-31T23:59:31Z"), To: parse("2026-11-01T00:01:31Z")}
	if err := plan11MatchEvidenceClockClaims("No evaluation points returned between 23:59:31Z and 00:00:31Z.", map[string]bool{}, crossing); err != nil {
		t.Fatal("verified UTC midnight prefix rejected", err)
	}
}
