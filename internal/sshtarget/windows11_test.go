package sshtarget

import (
	"encoding/json"
	"testing"
)

func TestWindowsClientAndServerEvidenceRemainDistinct(t *testing.T) {
	for _, test := range []struct {
		name           string
		product, build int
		admin          bool
		valid          bool
	}{
		{"windows11", 1, 26200, true, true},
		{"windows11_not_elevated", 1, 26200, false, false},
		{"windows10", 1, 19045, true, false},
		{"server2019", 3, 17763, true, true},
		{"server2022", 3, 20348, true, true},
		{"domain_controller", 2, 20348, true, true},
		{"old_server", 3, 14393, true, false},
		{"unknown_product", 0, 26200, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"product_type": test.product, "build_number": test.build, "architecture": "X64", "distribution_version": test.name, "service_manager": "windows_scm", "privileged": test.admin, "free_disk_bytes": 1 << 30, "identity_sid": "S-1-5-21-test"})
			got, err := parseWindowsEvidence(body)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
			if err == nil && (got.WindowsProductType != test.product || got.WindowsBuildNumber != test.build || got.Architecture != "amd64") {
				t.Fatalf("Windows identity lost: %+v", got)
			}
		})
	}
}
