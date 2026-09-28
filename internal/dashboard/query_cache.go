package dashboard

import "encoding/json"

// Only a trusted published entry point fills the revision identity. Draft
// validation and arbitrary query text never receive a reusable namespace.
func (runtime Runtime) targetCacheNamespace(panel Panel, target Target, variables, locals, inputs map[string]Selection, scope queryScope) string {
	if runtime.cacheDashboard == [16]byte{} || runtime.cacheRevision == [16]byte{} {
		return ""
	}
	encoded, err := json.Marshal(struct {
		Dashboard, Revision       [16]byte
		Panel                     Panel
		Target                    Target
		Variables, Locals, Inputs map[string]Selection
		Scope                     queryScope
		Compiler                  string
	}{runtime.cacheDashboard, runtime.cacheRevision, panel, target, variables, locals, inputs, scope, CompilerVersion})
	if err != nil {
		return ""
	}
	return queryHash(encoded)
}
