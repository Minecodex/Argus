package runner

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTimeoutTerminatesInterpreterAndRejectsReuse(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	b := testBinding(t)
	b.TimeoutSeconds = 1
	if _, err := manager.Open(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "Start-Sleep -Seconds 30", "sleep 30")
	start := time.Now()
	result, err := manager.Execute(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.SessionLost || !result.Terminated || result.ExitCode == 0 {
		t.Fatalf("timeout reported trustworthy success: %#v", result)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("timeout did not bound the process lifetime")
	}
	if _, err = manager.Open(t.Context(), b); err == nil {
		t.Fatal("timed out interpreter was reused")
	}
}

func TestInteractiveInputCannotConsumeNextApprovedCommand(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	b := testBinding(t)
	b.TimeoutSeconds = 2
	if _, err := manager.Open(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "Read-Host 'interactive input is not available'", "read -r value; test -n \"$value\"")
	result, err := manager.Execute(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("interactive command unexpectedly obtained input: %#v", result)
	}
}

func TestPowerShellNonTerminatingErrorRemainsFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires actual PowerShell")
	}
	manager := New(t.TempDir())
	defer manager.Close()
	b := testBinding(t)
	if _, err := manager.Open(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "Write-Error 'nonterminating marker' -ErrorAction Continue; Write-Output 'after error'", "")
	result, err := manager.Execute(t.Context(), b)
	if err != nil || result.ExitCode == 0 || !strings.Contains(string(result.Stderr), "nonterminating marker") || !strings.Contains(string(result.Stdout), "after error") || result.SessionLost {
		t.Fatalf("non-terminating error lost truthful exit/output/context: %#v %v", result, err)
	}
}
