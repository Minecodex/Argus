package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func testGateway(t *testing.T, count int) (*Gateway, *toolruntime.Set) {
	t.Helper()
	r := mcp.NewRegistry()
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("host.inspect_%04d", i)
		err := r.Register(mcp.Metadata{ID: id, Discovery: fixtureDiscovery(id), Risk: "read", Visibility: mcp.Visible, MaxResultBytes: 1024,
			Required: []string{"host.read"}, InputVersion: "1", OutputVersion: "1", InputSchema: object([]string{"limit"}, map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 10}}),
			Execute: func(_ context.Context, c mcp.Call) (mcp.Result, error) {
				return mcp.Result{Structured: map[string]any{"enterprise": c.Enterprise, "limit": c.Input["limit"]}}, nil
			}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Register(mcp.Metadata{ID: "host.update.commit", MaxResultBytes: 1024, Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		t.Fatal("hidden commit executed")
		return mcp.Result{}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	g, err := New(r, "test-implementation/v1")
	if err != nil {
		t.Fatal(err)
	}
	s, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: g.Revision}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	return g, s
}

func TestNativeDiscoveryBoundedAuthorizedAndConstantSchemas(t *testing.T) {
	var schemaBytes int
	for _, size := range []int{1, 500, 1000} {
		g, set := testGateway(t, size)
		encoded, _ := json.Marshal(set.Models())
		if schemaBytes != 0 && schemaBytes != len(encoded) {
			t.Fatalf("schema grows with catalog: %d -> %d", schemaBytes, len(encoded))
		}
		schemaBytes = len(encoded)
		t.Logf("native_catalog_tools=%d model_tools=%d core_schema_bytes=%d", size, len(set.Models()), len(encoded))
		p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"host.read"}}
		call := toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": "host"}}
		result, err := set.Invoke(context.Background(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		if count := len(result.Data["items"].([]map[string]any)); count != min(size, 10) {
			t.Fatalf("unbounded results: %d", count)
		}
		call.Principal.Permissions = nil
		result, err = set.Invoke(context.Background(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Data["items"].([]map[string]any)) != 0 {
			t.Fatal("unauthorized tool leaked")
		}
		call.Arguments = map[string]any{"category": "host", "name": "inspect_0000"}
		if _, err = set.Invoke(context.Background(), "tool.describe", call); err == nil {
			t.Fatal("unauthorized describe accepted")
		}
		call.Principal = p
		call.Arguments = map[string]any{"category": "host", "name": "update.commit", "arguments": map[string]any{}}
		if _, err = set.Invoke(context.Background(), "tool.invoke", call); err == nil {
			t.Fatal("hidden commit found")
		}
		if g.Revision == "" {
			t.Fatal("catalog has no revision")
		}
	}
}

func TestNativeInvocationValidatesSchemaAndTrustedIdentity(t *testing.T) {
	_, set := testGateway(t, 1)
	principal := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"host.read"}}
	call := toolruntime.Invocation{Principal: principal, Arguments: map[string]any{"category": "host", "name": "inspect_0000", "arguments": map[string]any{"limit": float64(11)}}}
	if _, err := set.Invoke(context.Background(), "tool.invoke", call); err == nil {
		t.Fatal("invalid input executed")
	}
	call.Arguments["arguments"] = map[string]any{"limit": float64(2)}
	result, err := set.Invoke(context.Background(), "tool.invoke", call)
	if err != nil {
		t.Fatal(err)
	}
	if result.Data["enterprise"] != principal.EnterpriseID.String() {
		t.Fatal("caller identity lost")
	}
	call.Arguments["enterprise_id"] = uuid.NewString()
	if _, err := set.Invoke(context.Background(), "tool.invoke", call); err == nil {
		t.Fatal("forged meta arguments accepted")
	}
}

func TestReadingAnActionDoesNotCreateAnotherConfirmationWait(t *testing.T) {
	r := mcp.NewRegistry()
	if err := r.Register(mcp.Metadata{ID: "workflow.action_status", Discovery: fixtureDiscovery("workflow.action_status"), Risk: "read", Visibility: mcp.Visible, InputVersion: "1", OutputVersion: "1", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		return mcp.Result{Structured: map[string]any{"action_ref": "act_existing", "status": "succeeded"}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	g, err := New(r, "status-test")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: g.Revision}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	result, err := set.Invoke(t.Context(), "tool.invoke", toolruntime.Invocation{Arguments: map[string]any{"category": "workflow", "name": "action_status", "arguments": map[string]any{}}})
	if err != nil || result.ActionRef != "" || result.Data["action_ref"] != "act_existing" {
		t.Fatalf("read-only action inspection requested confirmation: %+v %v", result, err)
	}
}

func fixtureDiscovery(name string) mcp.Discovery {
	return mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: "Inspect fixture " + name, Description: "Inspect an authorized synthetic host for catalog scale testing.", Keywords: []string{"inspect", "检查", "测试主机"}, ResultDescription: "Returns the fixture enterprise and limit.", Preconditions: []string{"Requires the fixture read permission."}, Examples: []mcp.DiscoveryExample{{Request: "Inspect a test host", Guidance: "Use a limit from 1 to 10."}}}
}
