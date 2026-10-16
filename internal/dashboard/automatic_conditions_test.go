package dashboard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
)

func TestAutomaticConditionsHaveProvenanceAndRespectUserCorrections(t *testing.T) {
	contracts := conditionContracts(EmptySpec())
	patch := dashboardparams.Patch{Time: &TimeRange{Kind: "relative", Seconds: 1800}}
	evidence := []dashboardparams.Evidence{{Path: "/time", Origin: "ai_selection", Reason: "Recent diagnostic window"}}
	initial, err := mergeConditions(dashboardparams.Empty(), patch, evidence, uuid.New(), "检查异常", contracts)
	if err != nil || initial.Evidence["/time"].Origin != "ai_selection" {
		t.Fatalf("AI provenance: %v", err)
	}
	patch.Time.Seconds = 3600
	corrected, err := mergeConditions(initial, patch, []dashboardparams.Evidence{{Path: "/time", Quote: "最近一小时"}}, uuid.New(), "改为最近一小时", contracts)
	if err != nil || corrected.Evidence["/time"].Origin != "message_interpretation" {
		t.Fatalf("correction: %v", err)
	}
	patch.Time.Seconds = 300
	if _, err = mergeConditions(corrected, patch, evidence, uuid.New(), "继续", contracts); err == nil {
		t.Fatal("AI silently replaced a user-defined range")
	}
	if _, err = mergeConditions(corrected, dashboardparams.Patch{ResetAll: true}, []dashboardparams.Evidence{{Path: "/", Origin: "ai_selection", Reason: "Reset for convenience"}}, uuid.New(), "继续", contracts); err == nil {
		t.Fatal("AI reset bypassed a user-defined range")
	}
	if _, err = mergeConditions(initial, patch, []dashboardparams.Evidence{{Path: "/time", Origin: "ai_selection"}}, uuid.New(), "继续", contracts); err == nil {
		t.Fatal("AI choice omitted its reason")
	}
}
