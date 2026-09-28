package contract_test

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/authorization"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

func TestFrontendPermissionRegistryMatchesBackend(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "packages", "api-client", "src", "generated", "permission-registry.ts"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)export const permissionIDs = (\[.*?\]) as const;`).FindSubmatch(raw)
	if len(match) != 2 {
		t.Fatal("generated permission registry is missing")
	}
	var actual []string
	if err = json.Unmarshal(match[1], &actual); err != nil {
		t.Fatal(err)
	}
	expected := []string{}
	for permission := range authorization.PermissionRegistry {
		expected = append(expected, permission)
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("frontend permission matrix is not generated from the current backend registry")
	}
}
