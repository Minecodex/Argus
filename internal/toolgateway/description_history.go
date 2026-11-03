package toolgateway

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type nativeDescriptionHistory interface {
	HasEarlierNativeToolDescription(context.Context, db.HasEarlierNativeToolDescriptionParams) (bool, error)
}

// Persisted successful category searches and descriptions are the authority
// across Worker restarts. The listed version must match the described contract.
// Prompt text, failed descriptions, another Run or the same model batch cannot
// establish that the model received this exact authorized tool schema.
func RequireNativeDescription(history nativeDescriptionHistory) func(context.Context, toolruntime.Invocation, Manifest) error {
	return func(ctx context.Context, call toolruntime.Invocation, manifest Manifest) error {
		missing := toolruntime.Error{Kind: "TOOL_DESCRIPTION_REQUIRED", Message: "Call tool.search for this category, wait for the complete authorized list, then call tool.describe for this exact name in a later round. Wait for that schema before invoking. Tool references from another result do not replace this sequence.", Details: map[string]any{"category": manifest.Category, "name": manifest.Name}}
		if history == nil || call.RunID == uuid.Nil || call.StepID == uuid.Nil || call.ModelCallID == uuid.Nil {
			return missing
		}
		found, err := history.HasEarlierNativeToolDescription(ctx, db.HasEarlierNativeToolDescriptionParams{
			EnterpriseID: call.Principal.EnterpriseID, UserID: call.Principal.UserID, RunID: call.RunID,
			StepID: call.StepID, ModelCallID: call.ModelCallID, Category: manifest.Category, Name: manifest.Name, Version: manifest.Version,
		})
		if err != nil {
			return fmt.Errorf("load authoritative native description: %w", err)
		}
		if !found {
			return missing
		}
		return nil
	}
}
