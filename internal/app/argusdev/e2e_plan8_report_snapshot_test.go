package argusdev

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPlan8StatusReportsRetainHashesWithoutChangingTestedImplementation(t *testing.T) {
	root := t.TempDir()
	runner := Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}
	for _, args := range [][]string{{"init", "-b", "codex/plan8-report-fixture"}, {"-c", "user.name=Plan8 fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "Snapshot fixture"}} {
		if err := runner.Run(t.Context(), nil, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "docs", "plan8")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"README.md", "progress.json", "57-completion-delivery.md", "09-acceptance-matrix.md"} {
		write(name, "initial")
	}
	app := App{root: root, runner: runner}
	initial, err := app.plan8Snapshot(t.Context())
	if err != nil || len(initial.ReportFiles) != 3 || initial.ReportFingerprint == "" {
		t.Fatal("status reports were not separately recorded", err)
	}
	previous := initial
	for _, name := range []string{"README.md", "progress.json", "57-completion-delivery.md"} {
		write(name, "updated observed status")
		current, err := app.plan8Snapshot(t.Context())
		if err != nil || current.Fingerprint != initial.Fingerprint || current.ReportFingerprint == previous.ReportFingerprint {
			t.Fatal("report changed tested implementation or lost its own identity", name, err)
		}
		previous = current
	}
	write("09-acceptance-matrix.md", "changed required assertions")
	changed, err := app.plan8Snapshot(t.Context())
	if err != nil || changed.Fingerprint == initial.Fingerprint {
		t.Fatal("acceptance contract change escaped source binding", err)
	}
}
