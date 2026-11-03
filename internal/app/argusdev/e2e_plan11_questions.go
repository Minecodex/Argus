package argusdev

import (
	"fmt"
	"slices"
	"strings"
)

func plan11ComparisonQuestions(selected []string) []string {
	if len(selected) == 0 {
		return []string{"metric", "catalog", "dashboard"}
	}
	return slices.Clone(selected)
}

func plan11ParseComparisonQuestions(raw string, suite, phase string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	if suite != "plan8" || phase != "plan11-compare" {
		return nil, fmt.Errorf("comparison questions require standalone plan8 plan11-compare")
	}
	selected := []string{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if !slices.Contains([]string{"metric", "catalog", "dashboard"}, name) || slices.Contains(selected, name) {
			return nil, fmt.Errorf("unknown, empty or repeated comparison question %q", name)
		}
		selected = append(selected, name)
	}
	return selected, nil
}

func plan11ParseComparisonProfiles(raw, suite, phase string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	if suite != "plan8" || phase != "plan11-compare" {
		return nil, fmt.Errorf("comparison profiles require standalone plan8 plan11-compare")
	}
	result := []string{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if !slices.Contains([]string{"candidate", "legacy"}, name) || slices.Contains(result, name) {
			return nil, fmt.Errorf("unknown, empty or repeated comparison profile %q", name)
		}
		result = append(result, name)
	}
	return result, nil
}
