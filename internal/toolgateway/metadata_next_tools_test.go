package toolgateway

import (
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestSDKEndpointNavigationKeepsCurrentPermissionAndTaskScope(t *testing.T) {
	gateway, err := New(monitoringDomainRegistry(t, &postgres.Store{}), "sdk-navigation-contract")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		permission, scope bool
		want              int
	}{{true, true, 1}, {false, true, 0}, {true, false, 0}} {
		principal := toolruntime.Principal{}
		if tc.permission {
			principal.Permissions = []string{"application.read"}
		}
		next := map[string]any{"items": []any{map[string]any{"category": "application", "name": "endpoint.list", "purpose": "Check SDK enabled metadata"}}}
		data := map[string]any{"items": []any{map[string]any{"sections": map[string]any{"next_tools": next}}}}
		gateway.filterCapabilityReferences(data, principal, func(string) bool { return tc.scope })
		if len(next["items"].([]any)) != tc.want {
			t.Fatal("SDK navigation reference granted a permission or escaped the Run scope", tc)
		}
	}
}
