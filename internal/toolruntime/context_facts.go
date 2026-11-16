package toolruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ContextRequest binds metadata to the same principal, Run and ledger boundary.
// Domain-authored facts never grant authority or prescribe an execution flow.
type ContextRequest struct {
	Principal       Principal
	RunID           string
	ThroughSequence int64
}

type ContextFactProvider interface {
	ContextFacts(context.Context, ContextRequest) (map[string]any, error)
}

func EmptyDiscoveryFacts(runID string) map[string]any {
	return map[string]any{"run_id": runID, "complete": true,
		"searched_categories": []map[string]any{}, "described_tools": []map[string]any{}, "recent_errors": []map[string]any{}, "recent_results": []map[string]any{},
		"semantics": "Current Run facts, not a grant. A prior Run summary does not satisfy this Run's Search/Describe. Correct failed fields using current contracts and facts; avoid unchanged repeated attempts."}
}

func ContextFacts(ctx context.Context, factory Factory, request ContextRequest) (map[string]any, error) {
	if provider, ok := factory.(ContextFactProvider); ok {
		return provider.ContextFacts(ctx, request)
	}
	return map[string]any{}, nil
}

func (factory CompositeFactory) ContextFacts(ctx context.Context, request ContextRequest) (map[string]any, error) {
	if factory.Validate != nil {
		if err := factory.Validate(ctx, request.Principal); err != nil {
			return nil, err
		}
	}
	result := map[string]any{}
	for _, source := range factory.Providers {
		provider, ok := source.(ContextFactProvider)
		if !ok {
			continue
		}
		facts, err := provider.ContextFacts(ctx, request)
		if err != nil {
			return nil, err
		}
		for name, value := range facts {
			if _, exists := result[name]; exists {
				return nil, fmt.Errorf("duplicate context fact provider: %s", name)
			}
			result[name] = value
		}
	}
	return result, nil
}

// BoundReferenceIndex retains whole records, never shortened paths or IDs.
func BoundReferenceIndex(items []map[string]any, complete bool, maxItems, maxBytes int) map[string]any {
	result := map[string]any{"schema_version": "argus.context_references/v1", "items": []map[string]any{}, "complete": complete,
		"semantics": "Exact registered references. Metadata is not proof of current contents or analysis. Read original data with existing tools; current access and content rules still apply."}
	kept := []map[string]any{}
	for _, item := range items {
		if len(kept) >= maxItems {
			result["complete"] = false
			break
		}
		candidate := append(append([]map[string]any{}, kept...), item)
		result["items"] = candidate
		raw, err := json.Marshal(result)
		if err != nil || len(raw) > maxBytes-384 {
			result["complete"] = false
			break
		}
		kept = candidate
	}
	result["items"] = kept
	if len(kept) < len(items) || !complete {
		result["complete"] = false
		result["recovery"] = "The index is partial. Retrieve original result metadata or inspect the authorized Workspace with existing tools. Omitted references may still exist."
	}
	raw, _ := json.Marshal(result)
	result["index_hash"] = fmt.Sprintf("%x", sha256.Sum256(raw))
	return result
}
