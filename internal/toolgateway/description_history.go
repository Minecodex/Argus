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

// Persisted successful descriptions are the authority across Worker restarts.
// Prompt text, failed descriptions, another Run or the same model batch cannot
// establish that the model received this exact authorized tool schema.
func RequireAlertDescription(history nativeDescriptionHistory) func(context.Context, toolruntime.Invocation, Manifest) error {
	return func(ctx context.Context, call toolruntime.Invocation, manifest Manifest) error {
		if manifest.Category != "alert" {
			return nil
		}
		missing := toolruntime.Error{Kind: "TOOL_DESCRIPTION_REQUIRED", Message: "Call tool.describe for this exact category/name and wait for its successful schema response in an earlier model round before invoking it.", Details: map[string]any{"category": manifest.Category, "name": manifest.Name}}
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
