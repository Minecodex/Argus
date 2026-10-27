package kuberneteshelm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHelmRendererDarwinMemoryLimitKillsOversizedProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "--argus-test-render-memory")
	command.Env = []string{"GOMAXPROCS=1", "GOMEMLIMIT=96MiB"}
	configureRenderCommand(command)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exit) {
		t.Fatalf("memory limit did not kill renderer: context=%v err=%v output=%s", ctx.Err(), err, output)
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || status.Signal() != syscall.SIGKILL || strings.Contains(string(output), "allocation-completed") {
		t.Fatalf("renderer did not hit the fatal memory limit: status=%v output=%s", exit.Sys(), output)
	}
}
