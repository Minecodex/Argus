package dashboard

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func allAnalysisResourcesForTest(input AnalysisResolveInput) AnalysisResolveInput {
	input.Changes.Resources = &dashboardparams.Resources{All: true, IDs: []uuid.UUID{}}
	input.Evidence = append(input.Evidence, dashboardparams.Evidence{Path: "/resources", Origin: "ai_selection", Reason: "Use the full authorized resource scope for this fixture scenario"})
	return input
}

func TestAnalysisCannotFallBackToImplicitAllAfterFailedResourceEvidence(t *testing.T) {
	base := dashboardparams.Empty()
	host := uuid.New()
	patch := dashboardparams.Patch{Resources: &dashboardparams.Resources{IDs: []uuid.UUID{host}}}
	event := uuid.New()
	_, err := mergeConditions(base, patch, []dashboardparams.Evidence{{Path: "/changes/resources", Quote: "this host"}}, event, "analyze this host", nil)
	var failure toolruntime.Error
	if !errors.As(err, &failure) || failure.Kind != "DASHBOARD_CONDITION_EVIDENCE_REQUIRED" || failure.Details["condition_evidence"] != true || !reflect.DeepEqual(failure.Details["required_paths"], []string{"/resources"}) {
		t.Fatalf("missing actionable public path feedback: %#v", err)
	}
	// The model's r29 repair removed the failed resource patch entirely.
	dropped, err := mergeConditions(base, dashboardparams.Patch{}, nil, event, "analyze this host", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = requireAnalysisResourceCondition(dropped); !errors.As(err, &failure) || failure.Kind != "DASHBOARD_RESOURCE_SCOPE_REQUIRED" || failure.Details["resource_condition_required"] != true {
		t.Fatalf("dropping the condition silently selected every resource: %v", err)
	}
	fixed, err := mergeConditions(base, patch, []dashboardparams.Evidence{{Path: "/resources", Quote: "this host"}}, event, "analyze this host", nil)
	if err != nil || requireAnalysisResourceCondition(fixed) != nil {
		t.Fatalf("corrected explicit resource choice rejected: %v", err)
	}
	if base.Overrides.Resources != nil || len(base.Evidence) != 0 {
		t.Fatal("failed or successful resolution mutated its inherited input")
	}
}

func TestAnalysisResourceChoicesInheritWithoutInventingConfirmation(t *testing.T) {
	for _, test := range []struct {
		name      string
		resources *dashboardparams.Resources
		path      string
		origin    string
		event     uuid.UUID
		allowed   bool
	}{
		{"default", nil, "", "", uuid.Nil, false},
		{"time_only", nil, "/time", "message_interpretation", uuid.New(), false},
		{"unattributed_ids", &dashboardparams.Resources{IDs: []uuid.UUID{uuid.New()}}, "", "", uuid.Nil, false},
		{"explicit_ids", &dashboardparams.Resources{IDs: []uuid.UUID{uuid.New()}}, "/resources", "message_interpretation", uuid.New(), true},
		{"explicit_empty", &dashboardparams.Resources{IDs: []uuid.UUID{}}, "/resources", "message_interpretation", uuid.New(), true},
		{"explicit_all", &dashboardparams.Resources{All: true, IDs: []uuid.UUID{}}, "/resources", "ai_selection", uuid.New(), true},
		{"resource_reset", nil, "/resources", "message_interpretation", uuid.New(), true},
		{"all_reset", nil, "/", "message_interpretation", uuid.New(), true},
		{"missing_event", nil, "/resources", "message_interpretation", uuid.Nil, false},
		{"automatic_reset", nil, "/resources", "candidate_reset", uuid.New(), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := dashboardparams.Empty()
			state.Overrides.Resources = test.resources
			if test.path != "" {
				state.Evidence[test.path] = dashboardparams.Reference{EventID: test.event, Origin: test.origin}
			}
			if err := requireAnalysisResourceCondition(state); (err == nil) != test.allowed {
				t.Fatalf("resource attribution: %v", err)
			}
		})
	}
}
