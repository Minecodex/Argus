package telemetry

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resourceref"
	"testing"
)

func TestObservationScopeSelectionIsExplicitAndTyped(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		name  string
		scope QueryScope
		valid bool
	}{
		{"explicit empty is not all", QueryScope{Mode: "explicit", Resources: []resourceref.Ref{}}, true},
		{"all requires no refs", QueryScope{Mode: "all_authorized", Resources: []resourceref.Ref{}}, true},
		{"typed host", QueryScope{Mode: "explicit", Resources: []resourceref.Ref{{Type: "host", ID: id}}}, true},
		{"no implicit mode", QueryScope{}, false},
		{"all with refs", QueryScope{Mode: "all_authorized", Resources: []resourceref.Ref{{Type: "host", ID: id}}}, false},
		{"ambiguous identity", QueryScope{Mode: "explicit", Resources: []resourceref.Ref{{Type: "host", ID: id}, {Type: "application", ID: id}}}, false},
		{"duplicate refs", QueryScope{Mode: "explicit", Resources: []resourceref.Ref{{Type: "host", ID: id}, {Type: "host", ID: id}}}, false},
		{"unknown resource", QueryScope{Mode: "explicit", Resources: []resourceref.Ref{{Type: "dashboard", ID: id}}}, false},
		{"unknown type filter", QueryScope{Mode: "all_authorized", ResourceTypes: []string{"dashboard"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.scope.Validate() == nil) != tc.valid {
				t.Fatalf("unexpected validation: %+v", tc.scope)
			}
		})
	}
}

func TestSupportedObservationMetricsRequireAnActualIntegration(t *testing.T) {
	recipes := []ObservationRecipe{{ID: "redis/memory", Version: 1, SourceType: "prometheus", IntegrationKey: "redis", Metrics: []string{"memory"}, Expression: "memory"}}
	sources := []ObservationSource{{SourceType: "prometheus", Keys: []string{"source:1"}, IntegrationKeys: []string{"other"}}}
	if got := supportedMetrics(recipes, sources, ""); len(got) != 0 {
		t.Fatal("generic Prometheus collection claimed Redis support")
	}
	sources[0].IntegrationKeys = []string{"redis"}
	got := supportedMetrics(recipes, sources, "")
	if len(got) != 1 || got[0].Observed || got[0].Type != "unknown" || got[0].Unit != "" {
		t.Fatalf("definition fabricated live metadata: %+v", got)
	}
}
