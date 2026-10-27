package toolgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestUndescribedArgumentsCannotBypassDiscoveryError(t *testing.T) {
	executed := 0
	registry := mcp.NewRegistry()
	if err := registry.Register(mcp.Metadata{ID: "alert.channel.list", Discovery: fixtureDiscovery("alert.channel.list"), Risk: "read", Visibility: mcp.Visible, Required: []string{"alert.read"}, InputVersion: "1", OutputVersion: "1", InputSchema: emptyObjectSchema(), MaxResultBytes: 1024, Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		executed++
		return mcp.Result{Structured: map[string]any{"items": []any{}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	gateway, err := New(registry, "description-before-business-schema")
	if err != nil {
		t.Fatal(err)
	}
	history := &descriptionHistoryStub{}
	gateway.RequireDescription = RequireNativeDescription(history)
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.read"}}, RunID: uuid.New(), StepID: uuid.New(), ModelCallID: uuid.New(), ID: uuid.New(), Arguments: map[string]any{"category": "alert", "name": "channel.list", "arguments": map[string]any{"guessed_field": true}}}
	assertCode := func(want string) {
		t.Helper()
		_, e := set.Invoke(t.Context(), "tool.invoke", call)
		var classified toolruntime.Error
		if !errors.As(e, &classified) || classified.Code() != want || executed != 0 {
			t.Fatalf("wrong pre-dispatch rejection: code=%s executed=%d", classified.Code(), executed)
		}
	}
	assertCode("TOOL_DESCRIPTION_REQUIRED")
	history.found = true
	assertCode("TOOL_INPUT_INVALID")
	history.found = false
	call.Principal.Permissions = nil
	assertCode("TOOL_NOT_FOUND")
	call.Principal.Permissions = []string{"alert.read"}
	history.err = errors.New("history unavailable")
	if _, err = set.Invoke(t.Context(), "tool.invoke", call); err == nil || executed != 0 {
		t.Fatal("description storage failure dispatched business")
	}
	history.err = nil
	history.found = true
	call.Arguments["arguments"] = map[string]any{}
	if _, err = set.Invoke(t.Context(), "tool.invoke", call); err != nil || executed != 1 {
		t.Fatal("described valid business request did not execute", err, executed)
	}
}
