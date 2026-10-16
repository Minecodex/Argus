package alerting

import (
	"context"
	"embed"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"regexp"
	"strings"
)

//go:embed templates/*.html
var presentationTemplates embed.FS

func publicPending(value db.PendingAction) map[string]any {
	preview := map[string]any{}
	diff := []any{}
	_ = json.Unmarshal(value.Preview, &preview)
	_ = json.Unmarshal(value.Diff, &diff)
	actions := []string{}
	if value.Status == "awaiting_confirmation" {
		actions = []string{"confirm", "cancel"}
	}
	return map[string]any{"schema_version": "argus.pending_action/v1", "action_ref": value.ActionRef, "action_type": value.ActionType, "title": value.Title, "summary": value.Summary, "risk": value.Risk, "status": value.Status, "preview": preview, "diff": diff, "available_actions": actions, "expires_at": value.ExpiresAt.Time, "created_at": value.CreatedAt.Time, "updated_at": value.UpdatedAt.Time}
}

func (t Tools) presentationMetadata(metadata mcp.Metadata) mcp.Metadata {
	if metadata.ID == "alert.event.get" {
		metadata.Project = projectInstanceTimeline
	}
	source, _ := presentationTemplates.ReadFile("templates/result.html")
	asset := presentation.ToolAsset(metadata.ID, string(source))
	metadata.Template = asset
	metadata.Present = func(ctx context.Context, call mcp.Call, result mcp.Result) (*toolruntime.Presentation, error) {
		e, err := uuid.Parse(call.Enterprise)
		if err != nil {
			return nil, err
		}
		u, err := uuid.Parse(call.Subject)
		if err != nil {
			return nil, err
		}
		scope, err := presentation.Scope(ctx, t.Service.Store, e, u)
		if err != nil {
			return nil, err
		}
		data := presentation.BindData(metadata.ID, metadata.Discovery.Title, result.Structured)
		data["request_context"] = presentation.PublicSnapshot(call.Input)
		refs := []toolruntime.ResourceRef{}
		names := []map[string]any{}
		seen := map[string]bool{}
		// Alerts with multiple participants remain gated by every underlying grant.
		if !strings.HasSuffix(metadata.ID, ".preview") {
			actor, err := t.actor(ctx, call)
			if err != nil {
				return nil, err
			}
			allowed, err := actorResources(ctx, t.Service.Store.Queries, actor)
			if err != nil {
				return nil, err
			}
			ids := []string{}
			collectAlertIDs(data, &ids)
			resourceIDs := []uuid.UUID{}
			for _, id := range ids {
				if seen[id] {
					continue
				}
				seen[id] = true
				uid, e := uuid.Parse(id)
				if e != nil {
					continue
				}
				if !containsAll(allowed, []uuid.UUID{uid}) {
					return nil, ErrDenied
				}
				resourceIDs = append(resourceIDs, uid)
			}
			names, refs, err = presentation.Resources(ctx, t.Service.Store, e, resourceIDs)
			if err != nil {
				return nil, err
			}
		}
		data["resources"] = names
		return presentation.Bound(asset, data, scope, refs), nil
	}
	return metadata
}
func collectAlertIDs(value any, refs *[]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "resource_ids" {
				if ids, ok := child.([]any); ok {
					for _, raw := range ids {
						if id, ok := raw.(string); ok {
							*refs = append(*refs, id)
						}
					}
				}
			} else {
				collectAlertIDs(child, refs)
			}
		}
	case []any:
		for _, child := range v {
			collectAlertIDs(child, refs)
		}
	}
}

func callEnterprise(call mcp.Call) uuid.UUID { id, _ := uuid.Parse(call.Enterprise); return id }

var publicField = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func previewSpec(raw json.RawMessage) any {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return map[string]any{}
	}
	return publicPreview(value)
}
func publicPreview(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range v {
			if key == "credential_id" {
				out["reference_id"] = child
				continue
			}
			if !publicField.MatchString(key) {
				continue
			}
			out[key] = publicPreview(child)
		}
		return out
	case []any:
		out := []any{}
		for _, child := range v {
			out = append(out, publicPreview(child))
		}
		return out
	default:
		return value
	}
}
