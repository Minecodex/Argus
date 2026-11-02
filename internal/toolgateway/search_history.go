package toolgateway

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type nativeSearchHistory interface {
	HasEarlierNativeToolSearch(context.Context, db.HasEarlierNativeToolSearchParams) (bool, error)
}

// Check before serving a cached Schema. A tool reference or an earlier schema
// from another Run cannot stand in for this Run's complete authorized listing.
func RequireNativeSearch(history nativeSearchHistory) func(context.Context, toolruntime.Invocation, Manifest) error {
	return func(ctx context.Context, call toolruntime.Invocation, manifest Manifest) error {
		missing := toolruntime.Error{Kind: "TOOL_SEARCH_REQUIRED", Message: "First call tool.search for this category and read its complete authorized list. In a later model round call tool.describe using this exact listed name; only then invoke after reading the schema. A tool reference is not a completed Search.", Details: map[string]any{"requires_search": true, "category": manifest.Category, "name": manifest.Name}}
		if history == nil || call.RunID == uuid.Nil || call.StepID == uuid.Nil || call.ModelCallID == uuid.Nil {
			return missing
		}
		found, err := history.HasEarlierNativeToolSearch(ctx, db.HasEarlierNativeToolSearchParams{EnterpriseID: call.Principal.EnterpriseID, UserID: call.Principal.UserID, RunID: call.RunID, StepID: call.StepID, ModelCallID: call.ModelCallID, Category: manifest.Category, Name: manifest.Name, Version: manifest.Version})
		if err != nil {
			return fmt.Errorf("load authoritative native search: %w", err)
		}
		if !found {
			return missing
		}
		return nil
	}
}
