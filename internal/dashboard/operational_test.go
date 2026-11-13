package dashboard

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestOperationalTemplateHasTypedReadOnlySourcesAndBoundedLayout(t *testing.T) {
	template := operationsOverviewTemplate()
	raw, _, err := validateTemplate(template)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := DecodeSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(spec)
	if !report.Valid {
		t.Fatalf("%+v", report)
	}
	if spec.SchemaVersion != SchemaVersionV2 || len(spec.Panels) != 12 {
		t.Fatal(spec.SchemaVersion, len(spec.Panels))
	}
	for _, panel := range spec.Panels {
		if !operationalPanel(panel) && !slices.Contains(sourceSignals[panel.SourceBinding.SourceType], panel.Signal) {
			t.Fatal("invalid telemetry capability in default template", panel.ID, panel.SourceBinding)
		}
		if operationalPanel(panel) && len(panel.Targets) != 0 {
			t.Fatal("arbitrary query on native panel")
		}
	}
}
func TestOperationalPanelsRejectLegacyAndArbitrarySources(t *testing.T) {
	for _, change := range []func(*Spec){
		func(s *Spec) { s.SchemaVersion = SchemaVersion },
		func(s *Spec) { s.Panels[0].DataSource.Kind = "https://foreign.example" },
		func(s *Spec) { s.Panels[0].DataSource.ResourceTypes = []string{"secret/private"} },
		func(s *Spec) { s.Panels[0].DataSource.Limit = 1000000 },
		func(s *Spec) { s.Panels[0].Targets = []Target{{ID: "unsafe"}} },
	} {
		spec := operationsOverviewTemplate().Spec
		change(&spec)
		if Validate(spec).Valid {
			raw, _ := json.Marshal(spec)
			t.Fatal("unsafe configuration accepted", string(raw))
		}
	}
}
