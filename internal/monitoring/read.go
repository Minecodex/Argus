package monitoring

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func applicationDTO(a db.MonitoringApplication) Application {
	return Application{ID: a.ID, Key: a.ApplicationKey, Name: a.Name, Environment: a.Environment, Description: a.Description, OwnerName: a.OwnerName, Status: a.Status, Version: a.Version, CreatedAt: a.CreatedAt.Time, UpdatedAt: a.UpdatedAt.Time}
}

func serviceDTO(s db.MonitoringApplicationService) ApplicationService {
	result := ApplicationService{ID: s.ID, ApplicationID: s.ApplicationID, Namespace: s.ServiceNamespace, Name: s.ServiceName, Runtime: s.Runtime, Status: s.Status, Version: s.Version}
	if s.FirstSeenAt.Valid {
		result.FirstSeenAt = &s.FirstSeenAt.Time
	}
	if s.LastSeenAt.Valid {
		result.LastSeenAt = &s.LastSeenAt.Time
	}
	return result
}

func (s Service) Applications(ctx context.Context, a Actor, in ListInput) (Page[Application], error) {
	in.Search = strings.TrimSpace(in.Search)
	out := Page[Application]{Items: []Application{}}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return out, err
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, s.Store.Queries, a, "application")
	if err != nil {
		return out, err
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "archived" {
		return out, ErrInvalid
	}
	limit, after, binding, err := s.page(a, "applications", in)
	if err != nil {
		return out, err
	}
	rows, err := s.Store.Queries.ListMonitoringApplications(ctx, db.ListMonitoringApplicationsParams{EnterpriseID: a.EnterpriseID, AuthorizedIds: ids, Status: in.Status, AfterID: after, Search: in.Search, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor, err = s.next(binding, rows[limit-1].ID)
		if err != nil {
			return out, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, applicationDTO(row))
	}
	if err = authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return Page[Application]{}, err
	}
	return out, nil
}

func (s Service) Application(ctx context.Context, a Actor, id uuid.UUID) (Application, error) {
	if s.Store == nil {
		return Application{}, ErrUnavailable
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return Application{}, err
	}
	if err := authorization.RequireResource(ctx, s.Store.Queries, a, "application", id); err != nil {
		return Application{}, err
	}
	row, err := s.Store.Queries.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return Application{}, lookupError(err)
	}
	if err = authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return Application{}, err
	}
	return applicationDTO(row), nil
}

func (s Service) Services(ctx context.Context, a Actor, in ListInput) (Page[ApplicationService], error) {
	in.Search = strings.TrimSpace(in.Search)
	out := Page[ApplicationService]{Items: []ApplicationService{}}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return out, err
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, s.Store.Queries, a, "application")
	if err != nil {
		return out, err
	}
	if in.ApplicationIDs != nil {
		for _, id := range in.ApplicationIDs {
			if !slices.Contains(ids, id) {
				return out, ErrDenied
			}
		}
		ids = in.ApplicationIDs
	}
	limit, after, binding, err := s.page(a, "application-services", in)
	if err != nil {
		return out, err
	}
	rows, err := s.Store.Queries.ListMonitoringServices(ctx, db.ListMonitoringServicesParams{EnterpriseID: a.EnterpriseID, ApplicationIds: ids, AfterID: after, Search: in.Search, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor, err = s.next(binding, rows[limit-1].ID)
		if err != nil {
			return out, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, serviceDTO(row))
	}
	if err = authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return Page[ApplicationService]{}, err
	}
	return out, nil
}

func (s Service) page(a Actor, audience string, in ListInput) (int, uuid.UUID, pagination.Binding, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 || len(in.Search) > 200 || len(in.Cursor) > 4096 || len(in.ApplicationIDs) > 1000 {
		return 0, uuid.Nil, pagination.Binding{}, ErrInvalid
	}
	filter := struct {
		Search, Status, Integration, ApplicationMode string
		ApplyStatus, Health, Freshness               string
		DesiredEnabled                               *bool
		ApplicationIDs                               []uuid.UUID
		Limit                                        int
	}{Search: strings.TrimSpace(in.Search), Status: in.Status, Integration: in.Integration, ApplicationMode: "all_authorized", ApplicationIDs: append([]uuid.UUID{}, in.ApplicationIDs...), Limit: limit,
		DesiredEnabled: in.DesiredEnabled, ApplyStatus: in.ApplyStatus, Health: in.Health, Freshness: in.Freshness}
	if in.ApplicationIDs != nil {
		filter.ApplicationMode = "explicit"
	}
	slices.SortFunc(filter.ApplicationIDs, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	b := pagination.Binding{Audience: "monitoring/" + audience, EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter(filter), Sort: "id_asc"}
	after := uuid.Nil
	if in.Cursor != "" {
		position, err := (pagination.Signer{Key: s.CursorKey, Now: s.Now}).Decode(in.Cursor, b)
		if err != nil {
			return 0, after, b, err
		}
		after, err = uuid.Parse(position.ID)
		if err != nil {
			return 0, uuid.Nil, b, ErrInvalid
		}
	}
	return limit, after, b, nil
}

func (s Service) next(binding pagination.Binding, id uuid.UUID) (string, error) {
	return (pagination.Signer{Key: s.CursorKey, Now: s.Now}).Encode(binding, pagination.Position{ID: id.String(), Time: time.Unix(1, 0).UTC()})
}
