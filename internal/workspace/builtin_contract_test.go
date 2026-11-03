package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/sandbox"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type builtinContractFixtureProvider struct {
	version  string
	executed *int
}

func (p builtinContractFixtureProvider) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	tools := []toolruntime.Tool{}
	for _, name := range []string{"read", "write", "edit", "bash"} {
		tools = append(tools, toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: name, Description: workspaceBuiltinDescription(name), Schema: toolSchema([]string{}, map[string]any{})}, Source: "sandbox", Version: p.version}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			*p.executed++
			return toolruntime.Result{}, nil
		}})
	}
	return toolruntime.Contribution{Tools: tools, SandboxStatus: "ready"}, nil
}

func TestChangedBuiltinContractCannotRunThroughOldSnapshot(t *testing.T) {
	identity := sandbox.WorkspaceIdentity{ProfileID: uuid.New(), ProfileRevision: 1, BackendID: uuid.New(), BackendVersion: 1, ImageID: uuid.New(), ImageVersion: 1, ImageDigest: "sha256:fixture"}
	executed := 0
	oldProvider := builtinContractFixtureProvider{identity.Version(), &executed}
	old, err := (toolruntime.CompositeFactory{Providers: []toolruntime.Provider{oldProvider}}).Build(t.Context(), toolruntime.Principal{})
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := old.Snapshot.Definitions[0].Version
	current := builtinContractFixtureProvider{workspaceBuiltinVersion(identity), &executed}
	restored, err := (toolruntime.CompositeFactory{Providers: []toolruntime.Provider{current}}).Restore(t.Context(), toolruntime.Principal{}, old.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Snapshot.Definitions[0].Version != oldVersion {
		t.Fatal("old Run definition was rewritten")
	}
	_, err = restored.Invoke(t.Context(), "write", toolruntime.Invocation{Arguments: map[string]any{}})
	var unavailable toolruntime.Error
	if !errors.As(err, &unavailable) || unavailable.Kind != "TOOL_VERSION_UNAVAILABLE" || executed != 0 {
		t.Fatal("changed builtin contract executed through an older Run snapshot", err, executed)
	}
}
