package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/kakj-go/Argus/internal/gen/openapi/conversationapi"
	"github.com/kakj-go/Argus/internal/gen/openapi/presentationapi"
)

func TestPresentationReferencesMatchSevenConversationTypes(t *testing.T) {
	want := []string{"application", "connector", "dashboard", "host", "kubernetes_cluster", "kubernetes_namespace", "monitoring_target"}
	for _, kind := range want {
		if !presentationapi.ToolPresentationResourceRefsType(kind).Valid() || !conversationapi.ConversationResourceRefType(kind).Valid() {
			t.Fatalf("generated public reference contract omits supported navigation type %q", kind)
		}
	}
	for _, kind := range []string{"", "secret", "credential", "managed_account", "model_governance", "unknown"} {
		if presentationapi.ToolPresentationResourceRefsType(kind).Valid() || conversationapi.ConversationResourceRefType(kind).Valid() {
			t.Fatalf("unrelated or private type accepted as public navigation: %q", kind)
		}
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "api", "openapi", "generated", "argus.bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle map[string]any
	if err = json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	schemas := bundle["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{"ToolPresentation", "ConversationResourceRef"} {
		schema := schemas[name].(map[string]any)
		if name == "ToolPresentation" {
			schema = schema["properties"].(map[string]any)["resource_refs"].(map[string]any)["items"].(map[string]any)
		}
		values := schema["properties"].(map[string]any)["type"].(map[string]any)["enum"].([]any)
		got := make([]string, 0, len(values))
		for _, value := range values {
			got = append(got, value.(string))
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s enum drifted from public navigation types: %v", name, got)
		}
	}
}
