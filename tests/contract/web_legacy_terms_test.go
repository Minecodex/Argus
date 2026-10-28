package contract_test

import (
	"regexp"
	"testing"
)

// Lezer's public syntax-token vocabulary is not the retired resource-label
// contract. Permit only this exact aliased import, never an arbitrary line or
// module containing legacy business fields.
var syntaxTokenImport = regexp.MustCompile(`^\s*import\s*\{\s*tags\s+as\s+syntaxTokens\s*\}\s*from\s*"@lezer/highlight";?\s*$`)

func legacyWebContractViolation(line string) bool {
	return !syntaxTokenImport.MatchString(line) && legacyPattern.MatchString(line)
}
func TestLegacyWebGuardSeparatesLibraryVocabularyFromResourceContracts(t *testing.T) {
	if legacyWebContractViolation(`import { tags as syntaxTokens } from "@lezer/highlight";`) {
		t.Fatal("syntax-token import is not a resource contract")
	}
	for _, line := range []string{`const resource = { tags: [] };`, `resourceGroupIds: []`, `import { tags as syntaxTokens } from "./resource-model";`, `import { tags as syntaxTokens } from "@lezer/highlight"; const tags = [];`} {
		if !legacyWebContractViolation(line) {
			t.Fatalf("legacy resource contract escaped: %s", line)
		}
	}
}
