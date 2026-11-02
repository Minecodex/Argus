package toolgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type searchHistoryStub struct {
	found  bool
	calls  int
	params db.HasEarlierNativeToolSearchParams
	err    error
}

func (s *searchHistoryStub) HasEarlierNativeToolSearch(_ context.Context, p db.HasEarlierNativeToolSearchParams) (bool, error) {
	s.calls++
	s.params = p
	return s.found, s.err
}

func TestDescribeRequiresEarlierSearchBeforeSchemaAndCache(t *testing.T) {
	gateway := discoveryFixtureGateway(t, fixtureDiscovery("early search"), false)
	history := &searchHistoryStub{}
	gateway.RequireSearch = RequireNativeSearch(history)
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New()}, RunID: uuid.New(), StepID: uuid.New(), ModelCallID: uuid.New(), Arguments: map[string]any{"category": "host", "name": "alpha"}}
	assertMissing := func() {
		t.Helper()
		result, err := set.Invoke(t.Context(), "tool.describe", call)
		var missing toolruntime.Error
		if !errors.As(err, &missing) || missing.Kind != "TOOL_SEARCH_REQUIRED" || len(result.Data) != 0 {
			t.Fatal("Describe served a schema before its Search", err)
		}
	}
	assertMissing()
	history.found = true
	for range 2 {
		if _, err := set.Invoke(t.Context(), "tool.describe", call); err != nil {
			t.Fatal(err)
		}
	}
	if history.calls != 3 {
		t.Fatal("cached Schema bypassed persisted Search authority")
	}
	if history.params.RunID != call.RunID || history.params.UserID != call.Principal.UserID || history.params.ModelCallID != call.ModelCallID || history.params.Name != "alpha" || history.params.Version == "" {
		t.Fatal("Search authority lost Run, subject or version binding")
	}
	history.found = false
	assertMissing()
	history.err = errors.New("private database failure")
	if _, err := set.Invoke(t.Context(), "tool.describe", call); err == nil {
		t.Fatal("history failure served a cached schema")
	}
}
