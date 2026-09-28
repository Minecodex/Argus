package argusdev

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestPlanV2RuntimeScopeCannotSkipOtherSuiteOrModelGates(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--suite", "m7", "--planv2-runtime-only"},
		{"run", "--suite", "planv2", "--planv2-runtime-only", "--unit-only"},
		{"run", "--suite", "planv2", "--planv2-runtime-only", "--real-model-config", "not-loaded.json"},
		{"run", "--suite", "m7", "--planv2-browser-grep", "chat"},
		{"run", "--suite", "planv2", "--planv2-runtime-only", "--planv2-browser-grep", "chat"},
		{"run", "--suite", "planv2", "--planv2-browser-grep", "[invalid"},
		{"run", "--suite", "planv2", "--planv2-real-model-only"},
		{"run", "--suite", "m7", "--planv2-real-model-only", "--real-model-config", "not-loaded.json"},
		{"run", "--suite", "planv2", "--planv2-real-model-only", "--unit-only", "--real-model-config", "not-loaded.json"},
		{"run", "--suite", "planv2", "--planv2-real-model-only", "--planv2-runtime-only", "--real-model-config", "not-loaded.json"},
		{"run", "--suite", "planv2", "--planv2-real-model-only", "--planv2-browser-grep", "chat", "--real-model-config", "not-loaded.json"},
	} {
		a := App{stdout: io.Discard, stderr: io.Discard}
		if err := a.runE2E(context.Background(), args); !errors.Is(err, errUsage) {
			t.Fatalf("invalid scope escaped validation: %v", err)
		}
	}
}
