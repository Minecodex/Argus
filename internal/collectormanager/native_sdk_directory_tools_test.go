package collectormanager_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func verifyNativeSDKApplicationDirectoryTools(t *testing.T, f *nativeSDKPipeline, registry *mcp.Registry, user uuid.UUID) func() {
	t.Helper()
	g, err := toolgateway.New(registry, "plan8-actual-sdk-directory")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: g.Revision}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	p := toolruntime.Principal{EnterpriseID: f.enterprise, UserID: user, Permissions: []string{"application.read"}}
	invoke := func(category, name string, args map[string]any) (toolruntime.Result, error) {
		t.Helper()
		if _, err := set.Invoke(f.ctx, "tool.describe", toolruntime.Invocation{ID: uuid.New(), Principal: p, Arguments: map[string]any{"category": category, "name": name}}); err != nil {
			return toolruntime.Result{}, err
		}
		return set.Invoke(f.ctx, "tool.invoke", toolruntime.Invocation{ID: uuid.New(), Principal: p, Arguments: map[string]any{"category": category, "name": name, "arguments": args}})
	}
	search, err := set.Invoke(f.ctx, "tool.search", toolruntime.Invocation{ID: uuid.New(), Principal: p, Arguments: map[string]any{"category": "application"}})
	if err != nil || len(search.Data["items"].([]map[string]any)) != 6 {
		t.Fatalf("application read catalog: %v", err)
	}
	other, err := set.Invoke(f.ctx, "tool.search", toolruntime.Invocation{ID: uuid.New(), Principal: p, Arguments: map[string]any{"category": "monitoring"}})
	if err != nil || len(other.Data["items"].([]map[string]any)) != 0 {
		t.Fatal("application access exposed monitoring tools")
	}
	apps, err := invoke("application", "list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	rows := apps.Data["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != f.apps[0].app.String() {
		t.Fatal("actual SDK directory crossed application")
	}
	services, err := invoke("application", "service.list", map[string]any{"application_ids": []any{f.apps[0].app.String()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(services.Data["items"].([]any)) != 2 {
		t.Fatal("SDK-discovered services absent from native directory")
	}
	if apps.Presentation == nil || len(apps.Presentation.Resources) != 1 {
		t.Fatal("SDK directory lost typed resource presentation")
	}
	if _, err = invoke("application", "get", map[string]any{"id": f.apps[1].app.String()}); err == nil {
		t.Fatal("native gateway read another SDK application")
	}
	scope := apps.Presentation.AuthorizationScope
	return func() {
		t.Helper()
		if _, err := invoke("application", "get", map[string]any{"id": f.apps[0].app.String()}); err == nil {
			t.Fatal("application native directory retained revoked access")
		}
		current, err := presentation.Scope(f.ctx, f.store, f.enterprise, user)
		if err != nil || scope == current {
			t.Fatal("SDK directory presentation ignored application revocation")
		}
		t.Log("real SDK-discovered applications/services use category search -> describe -> invoke, same-app scope, typed result and revoke invalidation")
	}
}
