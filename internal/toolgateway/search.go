package toolgateway

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalizeSearch(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(",，;；、", r) }), " ")
}

// English terms match word boundaries (install is not uninstall); Han terms
// match authored phrases so 创建 finds 创建主机. There is no business-name router.
func discoveryContains(text, term string) bool {
	text = strings.ToLower(text)
	han := strings.ContainsFunc(term, func(r rune) bool { return unicode.Is(unicode.Han, r) })
	for offset := 0; offset <= len(text)-len(term); {
		at := strings.Index(text[offset:], term)
		if at < 0 {
			return false
		}
		at += offset
		end := at + len(term)
		left, right := false, false
		if at > 0 {
			r, _ := utf8.DecodeLastRuneInString(text[:at])
			left = unicode.IsLetter(r) || unicode.IsDigit(r)
		}
		if end < len(text) {
			r, _ := utf8.DecodeRuneInString(text[end:])
			right = unicode.IsLetter(r) || unicode.IsDigit(r)
		}
		if han || !left && !right {
			return true
		}
		offset = at + len(term)
	}
	return false
}

func discoveryScore(m Manifest, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	score := 0
	if strings.EqualFold(m.Name, query) {
		score += 100
	}
	for _, term := range strings.Fields(query) {
		termScore := 0
		if discoveryContains(m.Name, term) {
			termScore += 10
		}
		if discoveryContains(m.Title, term) {
			termScore += 8
		}
		if discoveryContains(m.Description, term) {
			termScore++
		}
		keywordScore := 0
		for _, keyword := range m.Keywords {
			if keyword == term {
				keywordScore = 12
				break
			}
			if discoveryContains(keyword, term) {
				keywordScore = max(keywordScore, 6)
			}
		}
		termScore += keywordScore
		// Each term must match. Generic category words must not drown out a
		// requested action, and ties are sorted by canonical tool name.
		if termScore == 0 {
			return 0, false
		}
		score += termScore
	}
	return score, true
}

// Search is a bounded overview. Full business documentation is in Describe.
func discoverySummary(description string) string {
	text := []rune(description)
	if len(text) > 256 {
		return string(text[:255]) + "…"
	}
	return description
}
