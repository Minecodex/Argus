package alerting

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strings"
	"time"
)

func textFilter(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
func timeFilter(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*t)
}
func (s Service) listPosition(a Actor, f ListFilter, ids []uuid.UUID, collection string) (pagination.Binding, pagination.Position, int, error) {
	limit := f.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 500 || f.From != nil && f.To != nil && f.From.After(*f.To) {
		return pagination.Binding{}, pagination.Position{}, 0, ErrInvalid
	}
	f.Cursor = ""
	f.Limit = limit
	binding := pagination.Binding{Audience: "enterprise", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter([]any{f, ids}), Sort: "alert:" + collection + ":created_at_desc"}
	return binding, pagination.Position{}, limit, nil
}
func (s Service) decodeListCursor(token string, binding pagination.Binding) (pagination.Position, error) {
	if token == "" {
		return pagination.Position{}, nil
	}
	position, err := (pagination.Signer{Key: s.Actions.Key}).Decode(token, binding)
	if errors.Is(err, pagination.ErrAuthorizationVersionStale) {
		return position, ErrDenied
	}
	if err != nil {
		return position, ErrInvalid
	}
	return position, nil
}
func finishPage[T any](s Service, p *Page[T], binding pagination.Binding, more bool, at time.Time, id uuid.UUID) error {
	p.Complete = !more
	if !more {
		return nil
	}
	token, err := (pagination.Signer{Key: s.Actions.Key}).Encode(binding, pagination.Position{Time: at, ID: id.String()})
	if err != nil {
		return err
	}
	p.NextCursor = &token
	return nil
}
func (s Service) instancePage(ctx context.Context, a Actor, f ListFilter, ids []uuid.UUID, history bool) (Page[Instance], error) {
	page := Page[Instance]{Items: []Instance{}, ResourceIDs: ids, Evaluations: []EvaluationStatus{}}
	collection := "instances"
	if history {
		collection = "history"
	}
	binding, _, limit, err := s.listPosition(a, f, ids, collection)
	if err != nil {
		return page, err
	}
	position, err := s.decodeListCursor(f.Cursor, binding)
	if err != nil {
		return page, err
	}
	acknowledged := pgtype.Bool{}
	if f.Acknowledged != nil {
		acknowledged = pgtype.Bool{Bool: *f.Acknowledged, Valid: true}
	}
	count := db.CountFilteredAlertInstancesParams{EnterpriseID: a.EnterpriseID, AllowedResources: ids, Query: textFilter(f.Query), PolicyID: nullID(f.PolicyID), ResourceID: nullID(f.ResourceID), Severity: textFilter(f.Severity), Lifecycle: textFilter(f.Lifecycle), NotificationState: textFilter(f.NotificationState), Acknowledged: acknowledged, Since: timeFilter(f.From), Until: timeFilter(f.To), RuleID: nullID(f.RuleID), Metric: textFilter(f.Metric), History: history}
	total, err := s.Store.Queries.CountFilteredAlertInstances(ctx, count)
	if err != nil {
		return page, err
	}
	page.Total = int(total)
	rows, err := s.Store.Queries.PageAlertInstances(ctx, db.PageAlertInstancesParams{EnterpriseID: count.EnterpriseID, AllowedResources: count.AllowedResources, Query: count.Query, PolicyID: count.PolicyID, ResourceID: count.ResourceID, Severity: count.Severity, Lifecycle: count.Lifecycle, NotificationState: count.NotificationState, Acknowledged: count.Acknowledged, Since: count.Since, Until: count.Until, RuleID: count.RuleID, Metric: count.Metric, History: count.History, AfterTime: timestamp(position.Time), AfterID: nullID(uuidFromCursor(position.ID)), PageSize: int32(limit + 1)})
	if err != nil {
		return page, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, row := range rows {
		item, err := instanceDTO(row)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	page.Evaluations, err = s.readEvaluationStatuses(ctx, a, f)
	if err != nil {
		return page, err
	}
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		err = finishPage(s, &page, binding, more, last.CreatedAt.Time, last.ID)
	} else {
		page.Complete = true
	}
	return page, err
}
func uuidFromCursor(value string) uuid.UUID { id, _ := uuid.Parse(value); return id }
func (s Service) deliveryPage(ctx context.Context, a Actor, f ListFilter, ids []uuid.UUID) (Page[Delivery], error) {
	page := Page[Delivery]{Items: []Delivery{}, ResourceIDs: ids}
	binding, _, limit, err := s.listPosition(a, f, ids, "deliveries")
	if err != nil {
		return page, err
	}
	position, err := s.decodeListCursor(f.Cursor, binding)
	if err != nil {
		return page, err
	}
	count := db.CountFilteredAlertDeliveriesParams{EnterpriseID: a.EnterpriseID, AllowedResources: ids, Query: textFilter(f.Query), ChannelID: nullID(f.ChannelID), PolicyID: nullID(f.PolicyID), ResourceID: nullID(f.ResourceID), NotificationState: textFilter(f.NotificationState), Since: timeFilter(f.From), Until: timeFilter(f.To)}
	total, err := s.Store.Queries.CountFilteredAlertDeliveries(ctx, count)
	if err != nil {
		return page, err
	}
	page.Total = int(total)
	rows, err := s.Store.Queries.PageAlertDeliveries(ctx, db.PageAlertDeliveriesParams{EnterpriseID: count.EnterpriseID, AllowedResources: count.AllowedResources, Query: count.Query, ChannelID: count.ChannelID, PolicyID: count.PolicyID, ResourceID: count.ResourceID, NotificationState: count.NotificationState, Since: count.Since, Until: count.Until, AfterTime: timestamp(position.Time), AfterID: nullID(uuidFromCursor(position.ID)), PageSize: int32(limit + 1)})
	if err != nil {
		return page, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, row := range rows {
		item, err := deliveryDTO(row)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		err = finishPage(s, &page, binding, more, last.CreatedAt.Time, last.ID)
	} else {
		page.Complete = true
	}
	return page, err
}
func (s Service) readEvaluationStatuses(ctx context.Context, a Actor, f ListFilter) ([]EvaluationStatus, error) {
	items, err := s.Configurations(ctx, a, "policy")
	if err != nil {
		return nil, err
	}
	result := []EvaluationStatus{}
	for _, item := range items {
		if f.PolicyID != uuid.Nil && item.ID != f.PolicyID {
			continue
		}
		if f.Metric != "" {
			p, _ := decodeSpec[PolicySpec](item.Spec)
			found := false
			for _, r := range p.Rules {
				if strings.Contains(r.Expression, f.Metric) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		result = append(result, EvaluationStatus{PolicyID: item.ID, Name: item.Name, Health: item.Health, ErrorCode: item.ErrorCode, LastEvaluatedAt: item.LastEvaluatedAt, LastSuccessfulAt: item.LastSuccessfulAt})
	}
	return result, nil
}
