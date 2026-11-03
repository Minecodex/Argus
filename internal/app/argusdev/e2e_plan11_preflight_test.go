package argusdev

import (
	"errors"
	"testing"
)

func TestPlan11RequiresHostFixtureBeforeClusterMutation(t *testing.T) {
	app := &App{}
	for _, value := range []string{"", "   ", "--help", "image\nother"} {
		t.Setenv("ARGUS_P7_SYSTEMD_IMAGE", value)
		if err := app.checkSystemdFixturePrerequisite(t.Context(), "plan8"); !errors.Is(err, errCapability) {
			t.Fatalf("missing prerequisite did not fail before invoking commands: %v", err)
		}
	}
	if err := app.checkSystemdFixturePrerequisite(t.Context(), "m2"); err != nil {
		t.Fatal("unrelated suite gained fixture requirement", err)
	}
}
