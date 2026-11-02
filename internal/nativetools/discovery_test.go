package nativetools

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestProductionSearchDeliversAll133ToolsAcrossTwelveCategories(t *testing.T) {
	registry, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := toolgateway.New(registry, "production-discovery-test")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	principal := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"*"}}
	seen := map[string]bool{}
	categories := toolruntime.NativeCategoryNames()
	if len(categories) != 12 {
		t.Fatal("production category count changed", categories)
	}
	for _, category := range categories {
		call := toolruntime.Invocation{Principal: principal, Arguments: map[string]any{"category": category}}
		first, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil || first.Data["complete"] != true {
			t.Fatalf("incomplete production category %s: %v", category, err)
		}
		if _, exists := first.Data["next_cursor"]; exists {
			t.Fatal("tool category was paginated", category)
		}
		again, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil || !reflect.DeepEqual(first.Data, again.Data) {
			t.Fatal("production category order is unstable", category, err)
		}
		names := []string{}
		for _, item := range first.Data["items"].([]map[string]any) {
			name := item["name"].(string)
			key := category + "/" + name
			if seen[key] || strings.HasSuffix(name, ".commit") {
				t.Fatal("duplicate or hidden production tool", key)
			}
			seen[key] = true
			names = append(names, name)
			description, err := set.Invoke(t.Context(), "tool.describe", toolruntime.Invocation{Principal: principal, Arguments: map[string]any{"category": category, "name": name}})
			if err != nil {
				t.Fatal("listed production tool has no contract", key, err)
			}
			raw, _ := json.Marshal(description.Data)
			var document map[string]any
			if json.Unmarshal(raw, &document) != nil || document["name"] != name || document["version"] != item["version"] || document["input_schema"] == nil {
				t.Fatal("listed identity/version differs from its full contract", key)
			}
		}
		if !slices.IsSorted(names) {
			t.Fatal("production names are not sorted", category)
		}
	}
	if len(seen) != len(registry.ModelCatalog()) || len(seen) != 133 {
		t.Fatalf("public production entries did not all reach discovery: %d", len(seen))
	}
}
