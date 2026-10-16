package toolgateway

// Listing carries bounded summaries; Describe retains complete documentation.
func discoverySummary(description string) string {
	text := []rune(description)
	if len(text) > 256 {
		return string(text[:255]) + "…"
	}
	return description
}
