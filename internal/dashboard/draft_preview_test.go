package dashboard

import (
	"errors"
	"testing"
)

func TestScopedPreviewExcludesInvalidSiblingAndUnusedVariable(t *testing.T) {
	spec := EmptySpec()
	p := validPanel()
	p.Targets[0].SourceDefinition.Builder.Filters = []Filter{{Field: "environment", Operator: "=", Variable: "env"}}
	sibling := validPanel()
	sibling.ID = "invalid"
	sibling.Title = ""
	sibling.Targets = nil
	spec.Panels = []Panel{p, sibling}
	query := CandidateQuery{Signal: "metrics", SourceBinding: p.SourceBinding, Metric: "system_cpu_utilization", Field: "environment"}
	spec.Variables = []Variable{{ID: "env", Name: "env", Label: "Environment", Default: Selection{All: true}, IncludeAll: true, Query: query}, {ID: "unused", Name: "unused", Query: CandidateQuery{}}}
	if Validate(spec).Valid {
		t.Fatal("whole draft must remain invalid")
	}
	scoped, input, err := scopedPreviewSpec(spec, ExecutionInput{PanelIDs: []string{p.ID}, Variables: map[string]Selection{"env": {All: true}, "unused": {All: true}}})
	if err != nil || len(scoped.Panels) != 1 || len(scoped.Variables) != 1 || len(input.Variables) != 1 {
		t.Fatalf("incorrect execution closure: %v %+v", err, scoped)
	}
	if report := Validate(scoped); !report.Valid {
		t.Fatal(report.Issues)
	}
	for _, ids := range [][]string{{"missing"}, {p.ID, p.ID}} {
		if _, _, err := scopedPreviewSpec(spec, ExecutionInput{PanelIDs: ids}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid panel ids accepted: %v", ids)
		}
	}
	spec.Variables = append(spec.Variables, spec.Variables[0])
	if _, _, err := scopedPreviewSpec(spec, ExecutionInput{PanelIDs: []string{p.ID}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("ambiguous referenced variable was silently replaced")
	}
}

func TestDraftDefinitionHashPreservesPresentationAndInvalidatesQueries(t *testing.T) {
	spec := EmptySpec()
	spec.Panels = []Panel{validPanel()}
	before, err := draftDefinitionHash(spec, []string{"cpu"})
	if err != nil {
		t.Fatal(err)
	}
	spec.Panels[0].Title = "Renamed"
	spec.Panels[0].Type = "stat"
	spec.Panels[0].Unit = "percent"
	spec.Panels[0].Decimals = 2
	spec.Panels[0].Layout.Y = 8
	spec.Panels[0].Display = &DisplayOptions{Reducer: "last", Smooth: true}
	after, err := draftDefinitionHash(spec, []string{"cpu"})
	if err != nil || before != after {
		t.Fatal("presentation invalidated data context")
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Metric = "other_metric"
	after, err = draftDefinitionHash(spec, []string{"cpu"})
	if err != nil || after == before {
		t.Fatal("query edit reused old detail context")
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Metric = "system_cpu_utilization"
	spec.Panels[0].Drilldowns = []Drilldown{{ID: "changed", DetailQueryRef: "detail"}}
	after, err = draftDefinitionHash(spec, []string{"cpu"})
	if err != nil || after == before {
		t.Fatal("drilldown edit reused old detail context")
	}
}
