package argusdev

import (
	"errors"
	"io"
	"testing"
)

func TestPublicationMediaScopeRejectsUnrelatedOrExternalModelRuns(t *testing.T) {
	for _, args := range [][]string{
		{"e2e", "run", "--suite", "plan8", "--p5-publication-only"},
		{"e2e", "run", "--suite", "p5", "--unit-only", "--p5-publication-only"},
		{"e2e", "run", "--suite", "p5", "--p5-publication-only", "--real-model-config", "not-read.json"},
	} {
		err := (&App{stderr: io.Discard}).runE2E(t.Context(), args[1:])
		if !errors.Is(err, errUsage) {
			t.Fatal("focused publication test accepted wrong scope before environment setup", args, err)
		}
	}
}
