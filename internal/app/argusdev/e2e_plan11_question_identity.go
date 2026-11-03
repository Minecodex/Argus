package argusdev

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var plan11QuestionAbbreviation = regexp.MustCompile(`(?i)([0-9a-f][0-9a-f-]{7,35})(?:…|\.{3})([0-9a-f-]{0,36})`)
var plan11QuestionPrefix = regexp.MustCompile(`(?i)[0-9a-f]{8,32}`)

// A shortened label is usable only within the exact already-observed candidate
// set. This does not treat an arbitrary prefix as a full resource identity.
func plan11UniqueQuestionAbbreviation(content string, target uuid.UUID, candidates []uuid.UUID) bool {
	if target == uuid.Nil || len(candidates) == 0 {
		return false
	}
	unique := map[uuid.UUID]bool{}
	for _, candidate := range candidates {
		if candidate != uuid.Nil {
			unique[candidate] = true
		}
	}
	if !unique[target] {
		return false
	}
	for _, match := range plan11QuestionAbbreviation.FindAllStringSubmatchIndex(content, -1) {
		if match[0] > 0 && questionIdentityWordByte(content[match[0]-1]) || match[1] < len(content) && questionIdentityWordByte(content[match[1]]) {
			continue
		}
		prefix := strings.ToLower(strings.ReplaceAll(content[match[2]:match[3]], "-", ""))
		suffix := strings.ToLower(strings.ReplaceAll(content[match[4]:match[5]], "-", ""))
		if len(prefix) < 8 || len(prefix) > 32 || len(suffix) > 32 {
			continue
		}
		matches := []uuid.UUID{}
		for candidate := range unique {
			identity := strings.ReplaceAll(candidate.String(), "-", "")
			if strings.HasPrefix(identity, prefix) && strings.HasSuffix(identity, suffix) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 && matches[0] == target {
			return true
		}
	}
	for _, match := range plan11QuestionPrefix.FindAllStringIndex(content, -1) {
		if match[0] > 0 && questionIdentityWordByte(content[match[0]-1]) || match[1] < len(content) && questionIdentityWordByte(content[match[1]]) {
			continue
		}
		if strings.HasSuffix(content[:match[0]], "…") || strings.HasSuffix(content[:match[0]], "...") || strings.HasPrefix(content[match[1]:], "…") || strings.HasPrefix(content[match[1]:], "...") {
			continue
		}
		prefix := strings.ToLower(content[match[0]:match[1]])
		matches := []uuid.UUID{}
		for candidate := range unique {
			if strings.HasPrefix(strings.ReplaceAll(candidate.String(), "-", ""), prefix) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 && matches[0] == target {
			return true
		}
	}
	return false
}

func questionIdentityWordByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value == '-'
}
