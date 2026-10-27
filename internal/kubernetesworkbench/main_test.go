package kubernetesworkbench

import (
	"os"
	"testing"

	"github.com/kakj-go/Argus/internal/kuberneteshelm"
)

func TestMain(m *testing.M) {
	// Match server/worker/Connector entrypoints for native Helm integration
	// tests; the isolated child must never enter the test runner or load a DB.
	if kuberneteshelm.RunRenderCommand(os.Args[1:], os.Stdin, os.Stdout) {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
