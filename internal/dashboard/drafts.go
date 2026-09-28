package dashboard

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (service Service) ListDrafts(ctx context.Context, actor Actor) ([]db.DashboardDraft, error) {
	q := service.Store.Queries
	if err := authorize(ctx, q, actor, "telemetry.dashboard.manage"); err != nil {
		return nil, err
	}
	ids, err := authorizedIDs(ctx, q, actor, "dashboard")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDashboardDrafts(ctx, db.ListDashboardDraftsParams{EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID})
	if err != nil {
		return nil, err
	}
	result := []db.DashboardDraft{}
	for _, row := range rows {
		if !row.DashboardID.Valid || slices.Contains(ids, row.DashboardID.UUID) {
			result = append(result, row)
		}
	}
	return result, nil
}

func (service Service) DiscardDraft(ctx context.Context, actor Actor, id uuid.UUID, version int64) error {
	return service.draftTransaction(ctx, func(q *db.Queries) error {
		draft, err := loadDraft(ctx, q, actor, id, true)
		if err != nil {
			return err
		}
		rows, err := q.DiscardDashboardDraft(ctx, db.DiscardDashboardDraftParams{ID: id, EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID, DraftVersion: version})
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrConflict
		}
		return recordDraft(ctx, q, actor, draft, "dashboard.draft.discarded")
	})
}

// Rebase only changes an explicitly acknowledged baseline. It does not merge
// or replace the editor's content. Saving their resolved content is separate.
func (service Service) RebaseDraft(ctx context.Context, actor Actor, id uuid.UUID, version, objectVersion int64, revisionID uuid.UUID) (db.DashboardDraft, error) {
	var result db.DashboardDraft
	err := service.draftTransaction(ctx, func(q *db.Queries) error {
		draft, err := loadDraft(ctx, q, actor, id, true)
		if err != nil {
			return err
		}
		if !draft.DashboardID.Valid || draft.Status != "editing" || draft.DraftVersion != version {
			return ErrConflict
		}
		item, err := q.LockDashboard(ctx, db.LockDashboardParams{ID: draft.DashboardID.UUID, EnterpriseID: actor.EnterpriseID})
		if err != nil {
			return draftLookupError(err)
		}
		if item.Lifecycle != "active" {
			return ErrArchived
		}
		if item.Version != objectVersion || item.ActiveRevisionID.UUID != revisionID {
			return ErrConflict
		}
		if err := checkFolder(ctx, q, actor.EnterpriseID, draft.FolderID, true); err != nil {
			return err
		}
		result, err = q.RebaseDashboardDraft(ctx, db.RebaseDashboardDraftParams{ID: id, EnterpriseID: actor.EnterpriseID, EditorSubjectType: actor.SubjectType, EditorSubjectID: actor.SubjectID, BaseRevisionID: item.ActiveRevisionID, BaseObjectVersion: item.Version, DraftVersion: version})
		if err != nil {
			return translateConflict(err)
		}
		return recordDraft(ctx, q, actor, result, "dashboard.draft.rebased")
	})
	return result, translateConflict(err)
}

func (service Service) Revisions(ctx context.Context, actor Actor, id uuid.UUID) ([]db.DashboardRevision, error) {
	if _, _, err := service.Get(ctx, actor, id); err != nil {
		return nil, err
	}
	return service.Store.Queries.ListDashboardRevisions(ctx, db.ListDashboardRevisionsParams{DashboardID: id, EnterpriseID: actor.EnterpriseID})
}
