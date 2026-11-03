package observationscope

import (
	"slices"
	"testing"
)

func TestIntegrationFamilyIntersection(t *testing.T) {
	keys, filtered, err := discoveryIntegrations(DiscoveryInput{Families: []string{"database"}, Integrations: []string{"redis", "mysql"}})
	if err != nil || !filtered || !slices.Equal(keys, []string{"mysql"}) {
		t.Fatalf("intersection: %v %v %v", keys, filtered, err)
	}
	keys, filtered, err = discoveryIntegrations(DiscoveryInput{Families: []string{"database"}, Integrations: []string{"redis"}})
	if err != nil || !filtered || len(keys) != 0 {
		t.Fatal("empty intersection expanded")
	}
	for _, in := range []DiscoveryInput{{Families: []string{"invented"}}, {Integrations: []string{"invented"}}} {
		if _, _, err = discoveryIntegrations(in); err == nil {
			t.Fatal("accepted unknown discovery filter")
		}
	}
}
