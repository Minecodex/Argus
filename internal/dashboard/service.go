package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Service struct {
	Store    *postgres.Store
	Actions  resource.PendingActionService
	Verifier PublicationVerifier
}

// PublicationVerifier supplies current source/catalog/type/permission/budget
// validation separately from sample execution. A missing verifier fails closed
// for a nonempty dashboard, rather than claiming syntax checks prove validity.
type PublicationVerifier interface {
	Verify(context.Context, Actor, Spec) (ValidationReport, json.RawMessage, error)
}

type DraftInput struct {
	DashboardID      uuid.UUID       `json:"dashboard_id,omitempty"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	FolderID         uuid.UUID       `json:"folder_id,omitempty"`
	Spec             json.RawMessage `json:"spec"`
	ProposedBindings []Binding       `json:"proposed_bindings"`
	ExpectedVersion  int64           `json:"expected_version,omitempty"`
}

func (service Service) List(ctx context.Context, actor Actor) ([]db.Dashboard, error) {
	if err := authorize(ctx, service.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return nil, err
	}
	ids, err := authorizedIDs(ctx, service.Store.Queries, actor, "dashboard")
	if err != nil {
		return nil, err
	}
	rows, err := service.Store.Queries.ListDashboards(ctx, actor.EnterpriseID)
	if err != nil {
		return nil, err
	}
	result := []db.Dashboard{}
	for _, row := range rows {
		if slices.Contains(ids, row.ID) {
			result = append(result, row)
		}
	}
	return result, nil
}

func (service Service) Get(ctx context.Context, actor Actor, id uuid.UUID) (db.Dashboard, db.DashboardRevision, error) {
	if err := authorize(ctx, service.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return db.Dashboard{}, db.DashboardRevision{}, err
	}
	if err := requireObject(ctx, service.Store.Queries, actor, "dashboard", id); err != nil {
		return db.Dashboard{}, db.DashboardRevision{}, err
	}
	item, err := service.Store.Queries.GetDashboard(ctx, db.GetDashboardParams{ID: id, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		return item, db.DashboardRevision{}, ErrNotFound
	}
	revision, err := service.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: item.ActiveRevisionID.UUID, EnterpriseID: actor.EnterpriseID})
	return item, revision, err
}

func (service Service) CreateDraft(ctx context.Context, actor Actor, input DraftInput) (db.DashboardDraft, error) {
	return service.createDraft(ctx, actor, input, uuid.New(), false)
}

// A durable tool invocation may be recovered after its response was lost.
// Derive the initial personal draft identity from its immutable input, so a
// replay does not create a second draft. Later edits remain version checked.
func (service Service) CreateInvocationDraft(ctx context.Context, actor Actor, input DraftInput, invocation uuid.UUID) (db.DashboardDraft, error) {
	if invocation == uuid.Nil {
		return db.DashboardDraft{}, ErrInvalid
	}
	raw, err := json.Marshal([]any{actor.EnterpriseID, actor.SubjectID, invocation, input})
	if err != nil {
		return db.DashboardDraft{}, err
	}
	return service.createDraft(ctx, actor, input, uuid.NewSHA1(invocation, raw), true)
}
func (service Service) createDraft(ctx context.Context, actor Actor, input DraftInput, draftID uuid.UUID, replay bool) (db.DashboardDraft, error) {
	var result db.DashboardDraft
	err := service.draftTransaction(ctx, func(q *db.Queries) error {
		if err := authorize(ctx, q, actor, "telemetry.dashboard.manage"); err != nil {
			return err
		}
		if replay {
			prior, err := loadDraft(ctx, q, actor, draftID, false)
			if err == nil {
				result = prior
				return nil
			}
			if !errors.Is(err, ErrNotFound) && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		params := db.CreateDashboardDraftParams{ID: draftID, EnterpriseID: actor.EnterpriseID, DashboardID: nullID(input.DashboardID), EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID}
		if input.DashboardID != uuid.Nil {
			if err := requireObject(ctx, q, actor, "dashboard", input.DashboardID); err != nil {
				return err
			}
			item, err := q.LockDashboard(ctx, db.LockDashboardParams{ID: input.DashboardID, EnterpriseID: actor.EnterpriseID})
			if err != nil {
				return draftLookupError(err)
			}
			if item.Lifecycle != "active" {
				return ErrArchived
			}
			drafts, e := q.ListDashboardDrafts(ctx, db.ListDashboardDraftsParams{EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID})
			if e != nil {
				return e
			}
			for _, existing := range drafts {
				if existing.DashboardID == nullID(input.DashboardID) {
					result = existing
					return nil
				}
			}
			revision, err := q.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: item.ActiveRevisionID.UUID, EnterpriseID: actor.EnterpriseID})
			if err != nil {
				return err
			}
			params.BaseRevisionID = item.ActiveRevisionID
			params.BaseObjectVersion = item.Version
			input.Name = item.Name
			input.Description = item.Description
			input.FolderID = item.FolderID.UUID
			input.Spec = revision.Spec
		}
		if len(input.Spec) == 0 {
			input.Spec, _ = json.Marshal(EmptySpec())
		}
		if err := validateDraftInput(input); err != nil {
			return err
		}
		if err := checkFolder(ctx, q, actor.EnterpriseID, nullID(input.FolderID), true); err != nil {
			return err
		}
		params.Name = input.Name
		params.Description = input.Description
		params.FolderID = nullID(input.FolderID)
		params.Spec = input.Spec
		params.ProposedBindings, _ = json.Marshal(input.ProposedBindings)
		if input.ProposedBindings == nil {
			params.ProposedBindings = []byte("[]")
		}
		var err error
		result, err = q.CreateDashboardDraft(ctx, params)
		if err != nil {
			return err
		}
		return recordDraft(ctx, q, actor, result, "dashboard.draft.created")
	})
	return result, translateConflict(err)
}

func (service Service) Draft(ctx context.Context, actor Actor, id uuid.UUID) (db.DashboardDraft, error) {
	return loadDraft(ctx, service.Store.Queries, actor, id, false)
}

func loadDraft(ctx context.Context, q *db.Queries, actor Actor, id uuid.UUID, lock bool) (db.DashboardDraft, error) {
	var draft db.DashboardDraft
	if err := authorize(ctx, q, actor, "telemetry.dashboard.manage"); err != nil {
		return draft, err
	}
	var err error
	if lock {
		draft, err = q.LockDashboardDraft(ctx, db.LockDashboardDraftParams{ID: id, EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID})
	} else {
		draft, err = q.GetDashboardDraft(ctx, db.GetDashboardDraftParams{ID: id, EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID})
	}
	if err != nil {
		return draft, draftLookupError(err)
	}
	if draft.DashboardID.Valid {
		if err := requireObject(ctx, q, actor, "dashboard", draft.DashboardID.UUID); err != nil {
			return db.DashboardDraft{}, err
		}
	}
	return draft, nil
}

func (service Service) SaveDraft(ctx context.Context, actor Actor, id uuid.UUID, input DraftInput) (db.DashboardDraft, error) {
	var result db.DashboardDraft
	if err := validateDraftInput(input); err != nil {
		return result, err
	}
	err := service.draftTransaction(ctx, func(q *db.Queries) error {
		draft, err := loadDraft(ctx, q, actor, id, true)
		if err != nil {
			return err
		}
		if draft.Status != "editing" || draft.DraftVersion != input.ExpectedVersion {
			return ErrConflict
		}
		if input.DashboardID != uuid.Nil && nullID(input.DashboardID) != draft.DashboardID {
			return ErrInvalid
		}
		if err := checkFolder(ctx, q, actor.EnterpriseID, nullID(input.FolderID), true); err != nil {
			return err
		}
		bindings, _ := json.Marshal(input.ProposedBindings)
		if input.ProposedBindings == nil {
			bindings = []byte("[]")
		}
		result, err = q.SaveDashboardDraft(ctx, db.SaveDashboardDraftParams{ID: id, EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID, Name: input.Name, Description: input.Description, FolderID: nullID(input.FolderID), Spec: input.Spec, ProposedBindings: bindings, DraftVersion: input.ExpectedVersion})
		if err != nil {
			return translateConflict(err)
		}
		return recordDraft(ctx, q, actor, result, "dashboard.draft.saved")
	})
	return result, translateConflict(err)
}

func validateDraftInput(input DraftInput) error {
	if strings.TrimSpace(input.Name) == "" || len([]rune(input.Name)) > 120 || len(input.Description) > 8000 || len(input.ProposedBindings) > 100 {
		return ErrInvalid
	}
	if _, err := DecodeSpec(input.Spec); err != nil {
		return err
	}
	for _, binding := range input.ProposedBindings {
		if binding.ResourceID == uuid.Nil || binding.ResourceType != "host" && binding.ResourceType != "kubernetes_cluster" {
			return ErrInvalid
		}
	}
	return nil
}

func translateConflict(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	var sqlstate interface{ SQLState() string }
	if errors.As(err, &sqlstate) {
		switch sqlstate.SQLState() {
		case "23505":
			return ErrConflict
		case "40001", "40P01":
			return ErrUnavailable
		}
	}
	return err
}

func recordDraft(ctx context.Context, q *db.Queries, actor Actor, draft db.DashboardDraft, event string, extra ...map[string]any) error {
	details := draftAuditDetails(draft)
	for _, fields := range extra {
		for key, value := range fields {
			details[key] = value
		}
	}
	kind, id := "dashboard_draft", draft.ID
	if event == "dashboard.published" && draft.DashboardID.Valid {
		kind, id = "dashboard", draft.DashboardID.UUID
	}
	return appendDashboardAudit(ctx, q, actor, event, kind, id, draft.Name, "success", details)
}

func (service Service) verify(ctx context.Context, actor Actor, spec Spec) (ValidationReport, json.RawMessage, error) {
	report := Validate(spec)
	if !report.Valid {
		return report, nil, ErrInvalid
	}
	if len(spec.Panels) == 0 && len(spec.Variables) == 0 {
		return report, json.RawMessage(`{"status":"no_data","panels":[]}`), nil
	}
	if service.Verifier == nil {
		return report, nil, fmt.Errorf("%w: publication verifier unavailable", ErrUnavailable)
	}
	return service.Verifier.Verify(ctx, actor, spec)
}
