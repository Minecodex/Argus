package workspace

import (
	"mime"
	"testing"
)

func TestPublicationTitleDoesNotLoseWorkspaceFormat(t *testing.T) {
	for _, tc := range []struct{ path, name, explicit, expected string }{
		{"results/plan8-host-memory-report.md", "Plan8 host memory report (2026-11-02 02:51:29-02:56:29Z)", "", "text/markdown"},
		{"results/report.MD", "调查报告", "", "text/markdown"},
		{"results/report.csv", "Current observations", "", "text/csv"},
		{"results/report.json", "Evidence v1.unknown", "", "application/json"},
		{"results/report.md", "Evidence", "text/plain", "text/plain"},
		{"results/report.md", "Evidence.json", "", "application/json"},
		{"results/unknown", "Unclassified data", "", "application/octet-stream"},
	} {
		value := publicationMediaType(tc.path, tc.name, tc.explicit)
		base, _, err := mime.ParseMediaType(value)
		if err != nil || base != tc.expected {
			t.Fatalf("%q path=%q: %q, want %q: %v", tc.name, tc.path, value, tc.expected, err)
		}
	}
}
