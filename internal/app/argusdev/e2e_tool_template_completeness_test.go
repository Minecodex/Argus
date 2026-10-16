package argusdev

import "testing"

func TestTemplateAPMLimitFixturesMatchObservedQueryFacts(t *testing.T) {
	for _, state := range []string{"true", "false", "unknown"} {
		view := map[string]any{"key": "limit_" + state + "_queryAPMTopology"}
		if state != "unknown" {
			view["limited"] = state == "true"
		}
		if err := validateTemplateAPMLimitView(view); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateTemplateAPMLimitView(map[string]any{"key": "limit_true_queryAPMTopology", "limited": false}); err == nil {
		t.Fatal("do not proceed to browser acceptance with a mislabeled limited fixture")
	}
	if err := validateTemplateAPMLimitView(map[string]any{"key": "limit_unknown_queryAPMTopology", "limited": false}); err == nil {
		t.Fatal("an unknown fixture must omit the field")
	}
}
