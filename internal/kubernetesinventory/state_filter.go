package kubernetesinventory

import "slices"

// ValidStateFilter and MatchesState are shared by list and dashboard drilldowns.
func ValidStateFilter(state string) bool {
	return slices.Contains([]string{"", "healthy", "configured", "progressing", "pending", "warning", "failed", "completed", "paused", "scaled_down", "terminating", "unknown", "attention"}, state)
}
func MatchesState(object Object, state string) bool {
	if state == "" {
		return true
	}
	current := Summarize(object).State
	if state == "attention" {
		return slices.Contains([]string{"failed", "warning", "pending", "unknown"}, current)
	}
	return current == state
}
