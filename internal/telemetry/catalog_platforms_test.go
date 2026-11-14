package telemetry

import "testing"

func TestCatalogPlatformSelectionIsExplicitAndValidated(t *testing.T) {
	for _, selection := range [][]string{nil, {"linux_amd64"}, {"linux_amd64", "linux_arm64"}, {"windows_amd64"}} {
		if err := validateCatalogPlatforms(selection); err != nil {
			t.Fatal(selection, err)
		}
	}
	for _, selection := range [][]string{{""}, {"linux_amd64", "linux_amd64"}, {"linux"}, {"all"}} {
		if err := validateCatalogPlatforms(selection); err == nil {
			t.Fatal("invalid selection accepted", selection)
		}
	}
}
