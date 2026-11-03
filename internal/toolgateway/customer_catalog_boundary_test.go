package toolgateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestCustomerMCPToolsStayOutsideEveryNativeCategory(t *testing.T) {
	gateway, native := testGateway(t, 1)
	const alias = "mcp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_90b2f9ca2f1b"
	remoteCalls := 0
	tools := append(gateway.CoreTools(), toolruntime.Tool{
		Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: alias, Description: "Customer-owned tool.invoke", Schema: emptyObjectSchema()}, Source: "external_mcp", ConnectionID: uuid.NewString()},
		Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			remoteCalls++
			return toolruntime.Result{Data: map[string]any{"customer_data": true}}, nil
		},
	})
	combined, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, tools)
	if err != nil {
		t.Fatal(err)
	}
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"*"}}
	for _, category := range Categories {
		call := toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": category}}
		before, err := native.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		after, err := combined.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		x, _ := json.Marshal(before.Data)
		y, _ := json.Marshal(after.Data)
		if string(x) != string(y) || strings.Contains(string(y), alias) || strings.Contains(string(y), "Customer-owned") {
			t.Fatal("selected customer tool entered native category", category)
		}
		for _, name := range []string{alias, "tool.invoke", "update.commit"} {
			for _, method := range []string{"tool.describe", "tool.invoke"} {
				call.Arguments = map[string]any{"category": category, "name": name}
				if method == "tool.invoke" {
					call.Arguments["arguments"] = map[string]any{}
				}
				if _, err := combined.Invoke(t.Context(), method, call); err == nil {
					t.Fatal("native route accepted customer alias, same-name tool or hidden commit", category, name)
				}
			}
		}
	}
	if remoteCalls != 0 || len(combined.Models()) != len(native.Models())+1 {
		t.Fatal("native discovery forwarded customer invocation or changed top-level composition")
	}
	if _, err := combined.Invoke(t.Context(), alias, toolruntime.Invocation{Principal: p, Arguments: map[string]any{}}); err != nil || remoteCalls != 1 {
		t.Fatal("selected customer tool lost its independent direct route", err)
	}
	if combined.Snapshot.NativeCatalog != native.Snapshot.NativeCatalog {
		t.Fatal("customer contribution changed native catalog identity")
	}
}
