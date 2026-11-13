package dashboard

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strings"
)

type OverviewCandidatePage struct {
	Items      []db.Dashboard
	NextCursor string
}

func (s Service) OverviewCandidates(ctx context.Context, a Actor, cluster uuid.UUID, search, cursor string, limit int) (OverviewCandidatePage, error) {
	out := OverviewCandidatePage{Items: []db.Dashboard{}}
	if limit == 0 {
		limit = 50
	}
	search = strings.TrimSpace(search)
	if limit < 1 || limit > 100 || len(search) > 200 || len(cursor) > 32768 {
		return out, ErrInvalid
	}
	if err := authorize(ctx, s.Store.Queries, a, "telemetry.dashboard.read"); err != nil {
		return out, err
	}
	_, version, err := bindingResource(ctx, s.Store.Queries, a, "kubernetes_cluster", cluster, true, false)
	if err != nil {
		return out, err
	}
	ids, err := authorizedIDs(ctx, s.Store.Queries, a, "dashboard")
	if err != nil {
		return out, err
	}
	binding := pagination.Binding{Audience: "kubernetes_overview_candidates/v1", EnterpriseID: a.EnterpriseID.String(), SubjectID: a.SubjectID.String(), SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter([]any{cluster, version, search}), Sort: "id"}
	signer := pagination.Signer{Key: s.Actions.Key}
	after := uuid.Nil
	if cursor != "" {
		position, e := signer.Decode(cursor, binding)
		if e != nil {
			return out, ErrSelectionStale
		}
		after, e = uuid.Parse(position.ID)
		if e != nil {
			return out, ErrInvalid
		}
	}
	rows, err := s.Store.Queries.ListDashboardOverviewCandidates(ctx, db.ListDashboardOverviewCandidatesParams{EnterpriseID: a.EnterpriseID, DashboardIds: ids, Search: search, AfterID: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor, err = signer.Encode(binding, pagination.Position{ID: rows[len(rows)-1].ID.String()})
		if err != nil {
			return out, err
		}
	}
	out.Items = rows
	return out, nil
}
