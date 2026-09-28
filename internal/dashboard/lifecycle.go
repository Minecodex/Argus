package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type LifecycleInput struct {
	Operation       string    `json:"operation"`
	ID              uuid.UUID `json:"id"`
	ExpectedVersion int64     `json:"expected_version"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	SortOrder       int32     `json:"sort_order"`
}

func (service Service) Folders(ctx context.Context, actor Actor) ([]db.DashboardFolder, error) {
	if err := authorize(ctx, service.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return nil, err
	}
	return service.Store.Queries.ListDashboardFolders(ctx, actor.EnterpriseID)
}

func (service Service) PreviewLifecycle(ctx context.Context, actor Actor, input LifecycleInput, key string) (db.PendingAction, error) {
	if input.Operation == "folder.create" {
		if input.ID != uuid.Nil || input.ExpectedVersion != 0 {
			return db.PendingAction{}, ErrInvalid
		}
		if key == "" {
			return db.PendingAction{}, ErrInvalid
		}
		input.ID = uuid.NewSHA1(actor.EnterpriseID, []byte(actor.SubjectType+":"+actor.SubjectID.String()+":"+key+":folder"))
	}
	if err := validateLifecycle(ctx, service.Store.Queries, actor, input, false); err != nil {
		return db.PendingAction{}, err
	}
	kind := "dashboard"
	if strings.HasPrefix(input.Operation, "folder.") {
		kind = "dashboard_folder"
	}
	return service.Actions.PrepareForSubject(ctx, resource.ActionSubject{ID: actor.SubjectID.String(), Type: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion, ConfirmationRequired: true}, actor.EnterpriseID, resource.PrepareActionInput{
		ActionType: "telemetry.dashboard." + input.Operation, Title: input.Operation, Summary: "Update dashboard lifecycle", Risk: "write", ResourceType: kind, ResourceID: nullID(input.ID), ExpectedResourceVersion: pgtype.Int8{Int64: input.ExpectedVersion, Valid: input.ExpectedVersion > 0}, AuthorizationVersion: actor.AuthorizationVersion, Preview: input, Diff: []Change{{Kind: "change", Text: input.Operation + ": " + input.ID.String()}}, ImmutablePlan: input, ResourceScopeSnapshot: input, CommitHandler: "telemetry.dashboard.lifecycle.commit",
	}, key)
}

func validateLifecycle(ctx context.Context, q *db.Queries, actor Actor, input LifecycleInput, lock bool) error {
	if err := authorize(ctx, q, actor, "telemetry.dashboard.manage"); err != nil {
		return err
	}
	if input.ID == uuid.Nil {
		return ErrInvalid
	}
	switch input.Operation {
	case "folder.create", "folder.update", "folder.archive", "folder.restore":
		if input.Operation == "folder.create" || input.Operation == "folder.update" {
			if strings.TrimSpace(input.Name) == "" || len([]rune(input.Name)) > 120 || len(input.Description) > 8000 {
				return ErrInvalid
			}
		}
		if input.Operation == "folder.create" {
			if input.ExpectedVersion != 0 {
				return ErrInvalid
			}
			return nil
		}
		var folder db.DashboardFolder
		var err error
		if lock {
			folder, err = q.LockDashboardFolder(ctx, db.LockDashboardFolderParams{ID: input.ID, EnterpriseID: actor.EnterpriseID})
		} else {
			folder, err = q.GetDashboardFolder(ctx, db.GetDashboardFolderParams{ID: input.ID, EnterpriseID: actor.EnterpriseID})
		}
		if err != nil {
			return ErrNotFound
		}
		if folder.Version != input.ExpectedVersion {
			return ErrConflict
		}
		if input.Operation == "folder.restore" {
			if folder.Status != "archived" {
				return ErrConflict
			}
		} else if folder.Status != "active" {
			return ErrArchived
		}
		if input.Operation == "folder.archive" {
			count, err := q.CountFolderDashboards(ctx, db.CountFolderDashboardsParams{EnterpriseID: actor.EnterpriseID, FolderID: nullID(input.ID)})
			if err != nil {
				return err
			}
			if count != 0 {
				return ErrConflict
			}
		}
	case "archive", "restore":
		if err := requireObject(ctx, q, actor, "dashboard", input.ID); err != nil {
			return err
		}
		var item db.Dashboard
		var err error
		if lock {
			item, err = q.LockDashboard(ctx, db.LockDashboardParams{ID: input.ID, EnterpriseID: actor.EnterpriseID})
		} else {
			item, err = q.GetDashboard(ctx, db.GetDashboardParams{ID: input.ID, EnterpriseID: actor.EnterpriseID})
		}
		if err != nil {
			return ErrNotFound
		}
		if item.Version != input.ExpectedVersion {
			return ErrConflict
		}
		if input.Operation == "restore" {
			if item.Lifecycle != "archived" {
				return ErrConflict
			}
			return checkFolder(ctx, q, actor.EnterpriseID, item.FolderID, lock)
		}
		if item.Lifecycle != "active" {
			return ErrArchived
		}
	default:
		return ErrInvalid
	}
	return nil
}

func lifecycleState(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (LifecycleInput, Actor, error) {
	actor := Actor{EnterpriseID: action.EnterpriseID, SubjectID: action.CreatorSubjectID, SubjectType: action.CreatorSubjectType, AuthorizationVersion: action.AuthorizationVersion}
	var input LifecycleInput
	if json.Unmarshal(raw, &input) != nil || action.ActionType != "telemetry.dashboard."+input.Operation {
		return input, actor, ErrInvalid
	}
	return input, actor, validateLifecycle(ctx, q, actor, input, true)
}

func revalidateLifecycle(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) ([]byte, error) {
	input, _, err := lifecycleState(ctx, q, action, raw)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(encoded)
	return digest[:], nil
}

func commitLifecycle(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	input, actor, err := lifecycleState(ctx, q, action, raw)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	result := resource.ActionCommitResult{ResourceID: input.ID, ResourceType: "dashboard", Summary: input.Operation}
	if input.Operation == "archive" || input.Operation == "restore" {
		status := "archived"
		if input.Operation == "restore" {
			status = "active"
		}
		item, err := q.UpdateDashboardLifecycle(ctx, db.UpdateDashboardLifecycleParams{ID: input.ID, EnterpriseID: actor.EnterpriseID, Lifecycle: status, UpdatedBy: actor.SubjectID, Version: input.ExpectedVersion})
		result.ResourceVersion = item.Version
		if err != nil {
			return result, translateConflict(err)
		}
		return result, appendDashboardAudit(ctx, q, actor, "dashboard."+map[bool]string{true: "archived", false: "restored"}[input.Operation == "archive"], "dashboard", item.ID, item.Name, "success", map[string]any{"dashboard_id": item.ID, "revision_id": item.ActiveRevisionID.UUID, "before": map[string]any{"version": input.ExpectedVersion}, "after": map[string]any{"version": item.Version, "status": item.Lifecycle}, "action_ref": action.ActionRef})
	}
	result.ResourceType = "dashboard_folder"
	var folder db.DashboardFolder
	if input.Operation == "folder.create" {
		folder, err = q.CreateDashboardFolder(ctx, db.CreateDashboardFolderParams{ID: input.ID, EnterpriseID: actor.EnterpriseID, Name: input.Name, Description: input.Description, SortOrder: input.SortOrder, CreatedBy: actor.SubjectID})
	} else {
		folder, err = q.GetDashboardFolder(ctx, db.GetDashboardFolderParams{ID: input.ID, EnterpriseID: actor.EnterpriseID})
		if err != nil {
			return result, err
		}
		status := folder.Status
		name, description, order := folder.Name, folder.Description, folder.SortOrder
		switch input.Operation {
		case "folder.update":
			name = input.Name
			description = input.Description
			order = input.SortOrder
		case "folder.archive":
			status = "archived"
		case "folder.restore":
			status = "active"
		}
		folder, err = q.UpdateDashboardFolder(ctx, db.UpdateDashboardFolderParams{ID: input.ID, EnterpriseID: actor.EnterpriseID, Name: name, Description: description, SortOrder: order, Status: status, UpdatedBy: actor.SubjectID, Version: input.ExpectedVersion})
	}
	result.ResourceVersion = folder.Version
	if err != nil {
		return result, translateConflict(err)
	}
	event := map[string]string{"folder.create": "created", "folder.update": "updated", "folder.archive": "archived", "folder.restore": "restored"}[input.Operation]
	return result, appendDashboardAudit(ctx, q, actor, "dashboard.folder."+event, "dashboard_folder", folder.ID, folder.Name, "success", map[string]any{"folder_id": folder.ID, "before": map[string]any{"version": input.ExpectedVersion}, "after": map[string]any{"version": folder.Version, "status": folder.Status}, "action_ref": action.ActionRef})
}
