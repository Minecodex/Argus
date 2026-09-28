package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type publicationPlan struct {
	DraftID           uuid.UUID        `json:"draft_id"`
	DraftVersion      int64            `json:"draft_version"`
	DashboardID       uuid.NullUUID    `json:"dashboard_id"`
	BaseRevisionID    uuid.NullUUID    `json:"base_revision_id"`
	BaseObjectVersion int64            `json:"base_object_version"`
	SpecHash          string           `json:"spec_hash"`
	CompilerVersion   string           `json:"compiler_version"`
	FolderVersion     int64            `json:"folder_version"`
	Validation        ValidationReport `json:"validation"`
	Sample            json.RawMessage  `json:"sample"`
}

type PublicationPreview struct {
	Action     db.PendingAction `json:"action"`
	Validation ValidationReport `json:"validation"`
}

func (service Service) PreviewPublish(ctx context.Context, actor Actor, draftID uuid.UUID, expectedVersion int64, key string) (PublicationPreview, error) {
	draft, err := service.Draft(ctx, actor, draftID)
	if err != nil {
		return PublicationPreview{}, err
	}
	if draft.Status != "editing" || draft.DraftVersion != expectedVersion {
		return PublicationPreview{}, ErrConflict
	}
	if _, err := checkPublicationBase(ctx, service.Store.Queries, actor, draft, false); err != nil {
		return PublicationPreview{}, err
	}
	spec, err := DecodeSpec(draft.Spec)
	if err != nil {
		return PublicationPreview{}, err
	}
	report, sample, err := service.verify(ctx, actor, spec)
	preview := PublicationPreview{Validation: report}
	if err != nil || !report.Valid {
		if err == nil {
			err = ErrInvalid
		}
		return preview, err
	}
	plan := publicationPlan{DraftID: draft.ID, DraftVersion: draft.DraftVersion, DashboardID: draft.DashboardID, BaseRevisionID: draft.BaseRevisionID, BaseObjectVersion: draft.BaseObjectVersion, SpecHash: draftHash(draft), CompilerVersion: CompilerVersion, Validation: report, Sample: sample}
	if draft.FolderID.Valid {
		folder, e := service.Store.Queries.GetDashboardFolder(ctx, db.GetDashboardFolderParams{ID: draft.FolderID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil || folder.Status != "active" {
			return preview, ErrArchived
		}
		plan.FolderVersion = folder.Version
	}
	var before any
	if draft.BaseRevisionID.Valid {
		revision, e := service.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: draft.BaseRevisionID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			return preview, e
		}
		before = map[string]any{"name": revision.Name, "description": revision.Description, "folder_id": revision.FolderID, "spec": json.RawMessage(revision.Spec)}
	}
	after := map[string]any{"name": draft.Name, "description": draft.Description, "folder_id": draft.FolderID, "spec": spec, "bindings": json.RawMessage(draft.ProposedBindings)}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	action, err := service.Actions.PrepareForSubject(ctx, resource.ActionSubject{ID: actor.SubjectID.String(), Type: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion, ConfirmationRequired: true}, actor.EnterpriseID, resource.PrepareActionInput{
		RunID:      nullID(actor.RunID),
		ActionType: "telemetry.dashboard.publish", Title: draft.Name, Summary: "Publish dashboard revision", Risk: "write", ResourceType: "dashboard", ResourceID: draft.DashboardID,
		ExpectedResourceVersion: pgtype.Int8{Int64: draft.BaseObjectVersion, Valid: draft.DashboardID.Valid}, AuthorizationVersion: actor.AuthorizationVersion,
		Preview: map[string]any{"before_json": string(beforeJSON), "after_json": string(afterJSON), "validation": report, "sample_json": string(sample), "spec_hash": plan.SpecHash}, Diff: publicationDiff(before, after), ImmutablePlan: plan, ResourceScopeSnapshot: publicationScope(plan), CommitHandler: "telemetry.dashboard.publish.commit",
	}, key)
	preview.Action = action
	return preview, err
}

func draftHash(draft db.DashboardDraft) string {
	value, _ := json.Marshal(struct {
		Name, Description string
		Folder            uuid.NullUUID
		Spec, Bindings    json.RawMessage
	}{draft.Name, draft.Description, draft.FolderID, draft.Spec, draft.ProposedBindings})
	canonical, err := resource.CanonicalJSON(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func checkPublicationBase(ctx context.Context, q *db.Queries, actor Actor, draft db.DashboardDraft, lock bool) (db.Dashboard, error) {
	var item db.Dashboard
	if err := authorize(ctx, q, actor, "telemetry.dashboard.manage"); err != nil {
		return item, err
	}
	if draft.DashboardID.Valid {
		if err := requireObject(ctx, q, actor, "dashboard", draft.DashboardID.UUID); err != nil {
			return item, err
		}
		var err error
		if lock {
			item, err = q.LockDashboard(ctx, db.LockDashboardParams{ID: draft.DashboardID.UUID, EnterpriseID: actor.EnterpriseID})
		} else {
			item, err = q.GetDashboard(ctx, db.GetDashboardParams{ID: draft.DashboardID.UUID, EnterpriseID: actor.EnterpriseID})
		}
		if err != nil {
			return item, ErrNotFound
		}
		if item.Lifecycle != "active" {
			return item, ErrArchived
		}
		if item.Version != draft.BaseObjectVersion || item.ActiveRevisionID != draft.BaseRevisionID {
			return item, ErrConflict
		}
	}
	if err := checkFolder(ctx, q, actor.EnterpriseID, draft.FolderID, lock); err != nil {
		return item, err
	}
	var bindings []Binding
	if err := json.Unmarshal(draft.ProposedBindings, &bindings); err != nil {
		return item, ErrInvalid
	}
	for _, binding := range bindings {
		if _, _, err := bindingResource(ctx, q, actor, binding.ResourceType, binding.ResourceID, true, lock); err != nil {
			return item, err
		}
	}
	return item, nil
}

// ActionExtension participates in the existing private Action Executor. No
// HTTP route or model tool exposes CommitAction directly.
type ActionExtension struct {
	Next resource.ActionExtension
}

func (extension ActionExtension) RevalidateAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (hash []byte, err error) {
	defer func() {
		if strings.HasPrefix(action.ActionType, "telemetry.dashboard.") {
			err = actionFailure(err)
		}
	}()
	if action.ActionType != "telemetry.dashboard.publish" {
		if strings.HasPrefix(action.ActionType, "telemetry.dashboard.binding.") {
			return revalidateBinding(ctx, q, action, raw)
		}
		if strings.HasPrefix(action.ActionType, "telemetry.dashboard.") {
			return revalidateLifecycle(ctx, q, action, raw)
		}
		if extension.Next == nil {
			return nil, resource.ErrActionInvalidated
		}
		return extension.Next.RevalidateAction(ctx, q, action, raw)
	}
	plan, draft, actor, err := publicationState(ctx, q, action, raw)
	if err != nil {
		return nil, err
	}
	if _, err = checkPublicationBase(ctx, q, actor, draft, true); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(publicationScope(plan))
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	return digest[:], nil
}

func publicationScope(plan publicationPlan) any {
	return struct {
		DraftID  uuid.UUID
		Version  int64
		Hash     string
		Compiler string
	}{plan.DraftID, plan.DraftVersion, plan.SpecHash, plan.CompilerVersion}
}

func publicationState(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (publicationPlan, db.DashboardDraft, Actor, error) {
	actor := Actor{EnterpriseID: action.EnterpriseID, SubjectID: action.CreatorSubjectID, SubjectType: action.CreatorSubjectType, AuthorizationVersion: action.AuthorizationVersion}
	var plan publicationPlan
	if json.Unmarshal(raw, &plan) != nil || plan.CompilerVersion != CompilerVersion || !plan.Validation.Valid {
		return plan, db.DashboardDraft{}, actor, resource.ErrActionInvalidated
	}
	draft, err := loadDraft(ctx, q, actor, plan.DraftID, true)
	if err != nil {
		return plan, draft, actor, err
	}
	if draft.Status != "editing" || draft.DraftVersion != plan.DraftVersion || draft.DashboardID != plan.DashboardID || draft.BaseRevisionID != plan.BaseRevisionID || draft.BaseObjectVersion != plan.BaseObjectVersion || draftHash(draft) != plan.SpecHash {
		return plan, draft, actor, ErrConflict
	}
	if draft.FolderID.Valid {
		folder, e := q.LockDashboardFolder(ctx, db.LockDashboardFolderParams{ID: draft.FolderID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil || folder.Status != "active" || folder.Version != plan.FolderVersion {
			return plan, draft, actor, ErrConflict
		}
	}
	return plan, draft, actor, nil
}

func (extension ActionExtension) CommitAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (output resource.ActionCommitResult, err error) {
	defer func() {
		if strings.HasPrefix(action.ActionType, "telemetry.dashboard.") {
			err = actionFailure(err)
		}
	}()
	if action.ActionType != "telemetry.dashboard.publish" {
		if strings.HasPrefix(action.ActionType, "telemetry.dashboard.binding.") {
			return commitBinding(ctx, q, action, raw)
		}
		if strings.HasPrefix(action.ActionType, "telemetry.dashboard.") {
			return commitLifecycle(ctx, q, action, raw)
		}
		if extension.Next == nil {
			return resource.ActionCommitResult{}, resource.ErrActionInvalidated
		}
		return extension.Next.CommitAction(ctx, q, action, raw)
	}
	var result resource.ActionCommitResult
	plan, draft, actor, err := publicationState(ctx, q, action, raw)
	if err != nil {
		return result, err
	}
	item, err := checkPublicationBase(ctx, q, actor, draft, true)
	if err != nil {
		return result, err
	}
	revisionNumber := int64(1)
	if !draft.DashboardID.Valid {
		item, err = q.CreateDashboard(ctx, db.CreateDashboardParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, FolderID: draft.FolderID, Name: draft.Name, Description: draft.Description, CreatedBy: actor.SubjectID})
		if err != nil {
			return result, err
		}
		_, err = q.AddDataAuthorizationGrant(ctx, db.AddDataAuthorizationGrantParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, SubjectType: actor.SubjectType, SubjectID: actor.SubjectID, ResourceType: "dashboard", ResourceID: item.ID, CreatedBy: nullID(actor.SubjectID)})
		if err != nil {
			return result, err
		}
	} else {
		previous, e := q.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: item.ActiveRevisionID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			return result, e
		}
		revisionNumber = previous.RevisionNumber + 1
	}
	validation, _ := json.Marshal(plan.Validation)
	revision, err := q.CreateDashboardRevision(ctx, db.CreateDashboardRevisionParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, DashboardID: item.ID, RevisionNumber: revisionNumber, SchemaVersion: SchemaVersion, Name: draft.Name, Description: draft.Description, FolderID: draft.FolderID, Spec: draft.Spec, SpecHash: plan.SpecHash, ValidationReport: validation, SampleReport: plan.Sample, CreatedBy: actor.SubjectID})
	if err != nil {
		return result, err
	}
	item, err = q.ActivateDashboardRevision(ctx, db.ActivateDashboardRevisionParams{ID: item.ID, EnterpriseID: actor.EnterpriseID, ActiveRevisionID: nullID(revision.ID), Name: draft.Name, Description: draft.Description, FolderID: draft.FolderID, UpdatedBy: actor.SubjectID, Version: item.Version})
	if err != nil {
		return result, translateConflict(err)
	}
	draft, err = q.ConsumeDashboardDraft(ctx, db.ConsumeDashboardDraftParams{ID: draft.ID, EnterpriseID: actor.EnterpriseID, DashboardID: nullID(item.ID), PublishedRevisionID: nullID(revision.ID), DraftVersion: plan.DraftVersion})
	if err != nil {
		return result, translateConflict(err)
	}
	var bindings []Binding
	_ = json.Unmarshal(draft.ProposedBindings, &bindings)
	for _, binding := range bindings {
		created, e := q.CreateDashboardBinding(ctx, db.CreateDashboardBindingParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, DashboardID: item.ID, ResourceType: binding.ResourceType, ResourceID: binding.ResourceID, CreatedBy: actor.SubjectID})
		if e != nil {
			return result, e
		}
		if err = recordBinding(ctx, q, actor, created.ID, item.ID, binding.ResourceType, binding.ResourceID, item.Name, "attach", action.ActionRef); err != nil {
			return result, err
		}
	}
	if err := recordDraft(ctx, q, actor, draft, "dashboard.published", map[string]any{"action_ref": action.ActionRef, "revision_number": revision.RevisionNumber}); err != nil {
		return result, err
	}
	return resource.ActionCommitResult{ResourceType: "dashboard", ResourceID: item.ID, ResourceVersion: item.Version, Summary: fmt.Sprintf("Published dashboard revision %d", revisionNumber)}, nil
}

func actionFailure(err error) error {
	if errors.Is(err, ErrConflict) {
		return publicationConflict{err}
	}
	for _, permanent := range []error{ErrDenied, ErrConflict, ErrArchived, ErrNotFound, ErrInvalid} {
		if errors.Is(err, permanent) {
			return errors.Join(resource.ErrActionInvalidated, err)
		}
	}
	return err
}

type publicationConflict struct{ cause error }

func (e publicationConflict) Error() string                { return ErrConflict.Error() }
func (e publicationConflict) ActionValidationCode() string { return ErrConflict.Error() }
func (e publicationConflict) Unwrap() error {
	return errors.Join(resource.ErrActionInvalidated, e.cause)
}
