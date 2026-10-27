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

func TestNativeToolVersionFreezesSchemaPermissionsAndExecutionContract(t *testing.T) {
	version := func(change func(*mcp.Metadata)) string {
		t.Helper()
		meta := mcp.Metadata{ID: "host.inspect_contract", Discovery: fixtureDiscovery("host.inspect_contract"), Risk: "read", Visibility: mcp.Visible, ExecutionMode: mcp.ParallelSafe, MaxResultBytes: 1024, Required: []string{"host.read"}, InputVersion: "1", OutputVersion: "1", InputSchema: object([]string{}, map[string]any{"limit": map[string]any{"type": "integer", "maximum": 10}}), OutputSchema: map[string]any{"type": "object"}, Execute: func(context.Context, mcp.Call) (mcp.Result, error) { return mcp.Result{}, nil }}
		if change != nil {
			change(&meta)
		}
		reg := mcp.NewRegistry()
		if err := reg.Register(meta); err != nil {
			t.Fatal(err)
		}
		gateway, err := New(reg, "same-implementation")
		if err != nil {
			t.Fatal(err)
		}
		return gateway.manifests[0].Version
	}
	before := version(nil)
	for _, change := range []func(*mcp.Metadata){
		func(m *mcp.Metadata) {
			m.InputSchema["properties"].(map[string]any)["limit"].(map[string]any)["maximum"] = 20
		},
		func(m *mcp.Metadata) {
			m.OutputSchema = map[string]any{"type": "object", "required": []string{"items"}}
		},
		func(m *mcp.Metadata) { m.Required = []string{"host.manage"} },
		func(m *mcp.Metadata) { m.AnyRequired = []string{"host.manage"} },
		func(m *mcp.Metadata) { m.ExecutionMode = mcp.Sequential },
	} {
		if version(change) == before {
			t.Fatal("changed contract reused a previously described tool version")
		}
	}
	if version(nil) != before {
		t.Fatal("unchanged contract hash was unstable")
	}
}

func TestNativeDiscoveryCompleteAuthorizedAndConstantSchemas(t *testing.T) {
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
		if count := len(result.Data["items"].([]map[string]any)); count != size {
			t.Fatalf("incomplete category list: %d", count)
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

func TestPostActionCanPrepareNewPlanButCannotInvokeUnconfirmedMutation(t *testing.T) {
	r := mcp.NewRegistry()
	for _, id := range []string{"workflow.remote_plan.preview", "workflow.unapproved_change"} {
		id := id
		if err := r.Register(mcp.Metadata{ID: id, Discovery: fixtureDiscovery(id), Risk: "write", Visibility: mcp.Visible, InputVersion: "1", OutputVersion: "1", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
			if id != "workflow.remote_plan.preview" {
				t.Fatal("unconfirmed mutation invoked")
			}
			return mcp.Result{Structured: map[string]any{"action_ref": "act_new_preview"}}, nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	g, err := New(r, "post-action-plan")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: g.Revision}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	call := toolruntime.Invocation{ReadOnly: true, Arguments: map[string]any{"category": "workflow", "name": "remote_plan.preview", "arguments": map[string]any{}}}
	result, err := set.Invoke(t.Context(), "tool.invoke", call)
	if err != nil || result.ActionRef != "act_new_preview" {
		t.Fatalf("new preview blocked or consent lost: %#v %v", result, err)
	}
	call.Arguments["name"] = "unapproved_change"
	if _, err = set.Invoke(t.Context(), "tool.invoke", call); err == nil {
		t.Fatal("write bypass accepted")
	}
}
