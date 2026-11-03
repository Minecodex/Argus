package argusdev

import (
	"regexp"
	"strings"
	"time"
)

const plan11IntervalClock = `([0-2][0-9]:[0-5][0-9]:[0-5][0-9])Z?`

var plan11EmptyIntervalClaims = []*regexp.Regexp{
	regexp.MustCompile(plan11IntervalClock + `\s*(?:–|~|至|到|-)\s*` + plan11IntervalClock + `\s*(?:区间)?\s*未返回[^\n。;]*(?:评估点)`),
	regexp.MustCompile(`(?i)no (?:returned )?evaluation points(?: were)?(?: returned)? between\s+` + plan11IntervalClock + `\s+and\s+` + plan11IntervalClock),
}

// Only an explicitly stated empty prefix of returned evaluations is derived
// here. The caller has independently verified the primary and every panel's
// first point. This cannot attest raw sampling coverage or an arbitrary gap.
// Exempt only these two clock occurrences, never the same clock elsewhere.
func plan11VerifiedEmptyPrefixClock(text string, start, end int, prefixes []plan11ComparisonWindow) bool {
	for _, pattern := range plan11EmptyIntervalClaims {
		for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
			if !(start == match[2] && end == match[3] || start == match[4] && end == match[5]) {
				continue
			}
			left, right := text[match[2]:match[3]], text[match[4]:match[5]]
			for _, prefix := range prefixes {
				if prefix.From.IsZero() || !prefix.From.Before(prefix.To) || prefix.To.Sub(prefix.From) > 24*time.Hour {
					continue
				}
				for _, day := range []time.Time{prefix.From.UTC(), prefix.From.UTC().AddDate(0, 0, 1)} {
					from, e1 := time.Parse(time.RFC3339, day.Format("2006-01-02")+"T"+strings.TrimSuffix(left, "Z")+"Z")
					to, e2 := time.Parse(time.RFC3339, day.Format("2006-01-02")+"T"+strings.TrimSuffix(right, "Z")+"Z")
					if e1 != nil || e2 != nil {
						continue
					}
					if to.Before(from) {
						to = to.AddDate(0, 0, 1)
					}
					if !from.Before(prefix.From) && !to.Before(from) && to.Before(prefix.To) {
						return true
					}
				}
			}
		}
	}
	return false
}
