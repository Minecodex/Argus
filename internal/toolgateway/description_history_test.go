package toolgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type descriptionHistoryStub struct {
	found  bool
	err    error
	params db.HasEarlierNativeToolDescriptionParams
}

func (s *descriptionHistoryStub) HasEarlierNativeToolDescription(_ context.Context, params db.HasEarlierNativeToolDescriptionParams) (bool, error) {
	s.params = params
	return s.found, s.err
}

func TestAlertInvokeRequiresPersistedDescriptionBeforeBusiness(t *testing.T) {
	executed := 0
	registry := mcp.NewRegistry()
	if err := registry.Register(mcp.Metadata{ID: "alert.channel.list", Discovery: fixtureDiscovery("alert.channel.list"), Risk: "read", Visibility: mcp.Visible,
		Required: []string{"alert.read"}, InputVersion: "1", OutputVersion: "1", InputSchema: emptyObjectSchema(), MaxResultBytes: 1024,
		Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
			executed++
			return mcp.Result{Structured: map[string]any{"items": []any{}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	gateway, err := New(registry, "description-required")
	if err != nil {
		t.Fatal(err)
	}
	history := &descriptionHistoryStub{}
	gateway.RequireDescription = RequireAlertDescription(history)
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.read"}},
		RunID: uuid.New(), StepID: uuid.New(), ModelCallID: uuid.New(), ID: uuid.New(), Arguments: map[string]any{"category": "alert", "name": "channel.list", "arguments": map[string]any{}}}
	if _, err := set.Invoke(t.Context(), "tool.invoke", call); err == nil || executed != 0 {
		t.Fatal("undescribed business executed", executed, err)
	}
	history.found = true
	if _, err := set.Invoke(t.Context(), "tool.invoke", call); err != nil || executed != 1 {
		t.Fatal(executed, err)
	}
	if history.params.RunID != call.RunID || history.params.ModelCallID != call.ModelCallID || history.params.Name != "channel.list" || history.params.Version == "" {
		t.Fatal("description authority lost binding", history.params)
	}
	history.err = errors.New("storage unavailable")
	if _, err := set.Invoke(t.Context(), "tool.invoke", call); err == nil || executed != 1 {
		t.Fatal("history failure dispatched business", executed, err)
	}
}
