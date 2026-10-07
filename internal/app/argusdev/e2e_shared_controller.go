package argusdev

import (
	"fmt"
	"github.com/kakj-go/Argus/internal/app/argusctl"
)

// Reuse only an explicitly configured identity already trusted by an Argus
// install. Shared CRD schemas, deployment identity, ready Pods and imageID are
// still verified by argusctl. No foreign resources are adopted or modified.
func loadE2ESharedController(path string) (*argusctl.SharedSandboxController, error) {
	cfg, err := argusctl.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	pin := cfg.Spec.OpenSandbox.SharedController
	if pin == nil {
		return nil, fmt.Errorf("shared-controller-config has no immutable shared controller identity")
	}
	copy := *pin
	return &copy, nil
}
