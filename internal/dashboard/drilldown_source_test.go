package dashboard

import "testing"

func TestCorrelatedLogsPreserveSharedSourceIdentityWithoutConflatingCrossSourceRoutes(t *testing.T) {
	for _, mode := range []string{"builder", "dsl"} {
		for _, cross := range []bool{false, true} {
			spec := drillSpec(mode)
			var routes map[string]SourceBinding
			if cross {
				routes = map[string]SourceBinding{"logs": {SourceType: "filelog", CapabilityVersion: "v1"}}
			}
			generated, err := GenerateStandardDrilldowns(spec, "cpu", routes)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, drill := range generated.Spec.Panels[0].Drilldowns {
				if drill.Kind != "span_logs" {
					continue
				}
				found = true
				if (drill.Inputs["source_id"] == "/sourceId") == cross {
					t.Fatal("source correlation boundary lost")
				}
			}
			if !found {
				t.Fatal("missing span log drilldown")
			}
		}
	}
}
