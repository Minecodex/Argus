package main

import (
	"encoding/json"
	"fmt"
	"github.com/kakj-go/Argus/internal/authorization"
	"os"
	"sort"
)

func main() {
	path := "web/packages/api-client/src/generated/permission-registry.ts"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	keys := make([]string, 0, len(authorization.PermissionRegistry))
	for key := range authorization.PermissionRegistry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	roles := map[string][]string{}
	for _, role := range authorization.BuiltinRoles {
		values := append([]string{}, role.Permissions...)
		sort.Strings(values)
		roles[role.Key] = values
	}
	ids, _ := json.MarshalIndent(keys, "", "  ")
	builtins, _ := json.MarshalIndent(roles, "", "  ")
	text := fmt.Sprintf("/** Generated from internal/authorization. DO NOT EDIT. */\nexport const permissionRegistryVersion = %d;\nexport const permissionIDs = %s as const;\nexport const builtinRolePermissions: Record<string, readonly string[]> = %s;\n", authorization.PermissionRegistryVersion, ids, builtins)
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		panic(err)
	}
}
