package argusdev

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPlan7LinuxChatboxRequiresExplicitRealModelBeforePreparingResources(t *testing.T) {
	a := App{stdout: io.Discard, stderr: io.Discard}
	err := a.runE2E(t.Context(), []string{"run", "--suite", "plan7", "--plan7-phase", "linux-chatbox"})
	if !errors.Is(err, errUsage) || !strings.Contains(err.Error(), "requires --real-model-config") {
		t.Fatalf("Linux Chatbox gate did not reject missing real provider before resource setup: %v", err)
	}
	err = a.runE2E(t.Context(), []string{"run", "--suite", "plan6", "--plan7-phase", "linux-chatbox"})
	if !errors.Is(err, errUsage) || !strings.Contains(err.Error(), "invalid Plan7 phase") {
		t.Fatalf("focused gate escaped suite isolation: %v", err)
	}
}
