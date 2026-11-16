package dashboard

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestConditionEvidencePinpointsInvalidQuoteAndDuplicatePaths(t *testing.T) {
	patch := dashboardparams.Patch{Time: &TimeRange{Kind: "relative", Seconds: 300}}
	_, err := mergeConditions(dashboardparams.Empty(), patch, []dashboardparams.Evidence{{Path: "/time", Quote: "paraphrased time"}}, uuid.New(), "exact current time", nil)
	var failure toolruntime.Error
	if !errors.As(err, &failure) {
		t.Fatal("missing evidence feedback", err)
	}
	issues := failure.Details["issues"].([]map[string]string)
	if issues[0]["path"] != "/evidence/0/quote" || issues[0]["constraint"] != "exact_excerpt_from_current_user_message" {
		t.Fatal("quote failure was generic", issues)
	}
	_, err = mergeConditions(dashboardparams.Empty(), patch, []dashboardparams.Evidence{{Path: "/time", Quote: "exact"}, {Path: "/time", Quote: "exact"}}, uuid.New(), "exact current time", nil)
	if !errors.As(err, &failure) || failure.Details["issues"].([]map[string]string)[0]["constraint"] != "one_entry_per_changed_condition" {
		t.Fatal("duplicate evidence not explained", err)
	}
}
