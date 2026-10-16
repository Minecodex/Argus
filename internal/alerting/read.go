package alerting

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func instanceDTO(row db.AlertInstance) (Instance, error) {
	var state InstanceState
	if err := json.Unmarshal(row.State, &state); err != nil {
		return Instance{}, err
	}
	var err error
	state, err = projectInstanceSnapshot(state, row.ResourceIds, row.SourceIds)
	if err != nil {
		return Instance{}, err
	}
	result := Instance{ID: row.ID, PolicyID: row.PolicyID, PolicyVersion: row.PolicyVersion, ResourceIDs: row.ResourceIds, SourceIDs: row.SourceIds, State: state, Lifecycle: row.Lifecycle, Health: row.Health, Severity: row.Severity, HandlingVersion: row.HandlingVersion, Acknowledged: row.Acknowledged, StartedAt: timePtr(row.StartedAt), ResolvedAt: timePtr(row.ResolvedAt), LastEvaluatedAt: row.LastEvaluatedAt.Time, Version: row.Version}
	if row.OwnerUserID.Valid {
		result.OwnerID = &row.OwnerUserID.UUID
	}
	return result, nil
}
func deliveryDTO(row db.AlertDelivery) (Delivery, error) {
	var message Message
	if err := json.Unmarshal(row.Payload, &message); err != nil {
		return Delivery{}, err
	}
	message.Initiator = nil
	v := Delivery{ID: row.ID, ChannelID: row.ChannelID, ResourceIDs: row.ResourceIds, Message: message, Status: row.Status, Attempt: row.Attempt, LastError: row.LastError, LastAttemptAt: timePtr(row.LastAttemptAt), CreatedAt: row.CreatedAt.Time, Version: row.Version}
	if row.BatchID.Valid {
		v.BatchID = &row.BatchID.UUID
	}
	if row.ResendOfID.Valid {
		v.ResendOfID = &row.ResendOfID.UUID
	}
	if row.InstanceID.Valid {
		v.InstanceID = &row.InstanceID.UUID
	}
	return v, nil
}
func (s Service) Instances(ctx context.Context, a Actor, f ListFilter, history bool) (Page[Instance], error) {
	empty := Page[Instance]{Items: []Instance{}}
	if err := authorize(ctx, s.Store.Queries, a, "alert.read"); err != nil {
		return empty, err
	}
	ids, err := actorResources(ctx, s.Store.Queries, a)
	if err != nil {
		return empty, err
	}
	return s.instancePage(ctx, a, f, ids, history)
}

func filterInstance(i Instance, f ListFilter) bool {
	if f.RuleID != uuid.Nil && i.State.RuleID != f.RuleID {
		found := false
		for _, member := range i.State.Members {
			if member.RuleID == f.RuleID {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if f.PolicyID != uuid.Nil && f.PolicyID != i.PolicyID || f.ResourceID != uuid.Nil && !slices.Contains(i.ResourceIDs, f.ResourceID) || f.Severity != "" && f.Severity != i.Severity || f.Lifecycle != "" && f.Lifecycle != i.Lifecycle || f.NotificationState != "" && f.NotificationState != i.State.NotificationState || f.Acknowledged != nil && *f.Acknowledged != i.Acknowledged {
		return false
	}
	if f.From != nil && i.LastEvaluatedAt.Before(*f.From) || f.To != nil && i.LastEvaluatedAt.After(*f.To) {
		return false
	}
	if f.Metric != "" && !strings.Contains(i.State.Metric, f.Metric) {
		return false
	}
	if f.Query != "" {
		value, _ := json.Marshal(i)
		if !strings.Contains(strings.ToLower(string(value)), strings.ToLower(f.Query)) {
			return false
		}
	}
	return true
}

func (s Service) Instance(ctx context.Context, a Actor, id uuid.UUID) (Instance, []Event, error) {
	if err := authorize(ctx, s.Store.Queries, a, "alert.read"); err != nil {
		return Instance{}, nil, err
	}
	row, err := s.Store.Queries.GetAlertInstance(ctx, db.GetAlertInstanceParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return Instance{}, nil, ErrNotFound
	}
	ids, err := actorResources(ctx, s.Store.Queries, a)
	if err != nil || !containsAll(ids, row.ResourceIds) {
		return Instance{}, nil, ErrDenied
	}
	result, err := instanceDTO(row)
	if err != nil {
		return result, nil, err
	}
	rows, err := s.Store.Queries.ListAlertEvents(ctx, db.ListAlertEventsParams{EnterpriseID: a.EnterpriseID, InstanceID: id})
	events := []Event{}
	for _, e := range rows {
		payload := e.Payload
		var historical struct {
			State     *InstanceState `json:"state"`
			SourceIDs []uuid.UUID    `json:"source_ids"`
		}
		if json.Unmarshal(payload, &historical) != nil {
			return result, nil, ErrInvalid
		}
		if historical.State != nil {
			var snapshot Instance
			if json.Unmarshal(payload, &snapshot) != nil {
				return result, nil, ErrInvalid
			}
			snapshot.State, err = projectInstanceSnapshot(*historical.State, e.ResourceIds, historical.SourceIDs)
			if err != nil {
				return result, nil, err
			}
			payload, err = json.Marshal(snapshot)
			if err != nil {
				return result, nil, err
			}
		}
		v := Event{ID: e.ID, InstanceID: e.InstanceID, EventType: e.EventType, Payload: payload, Origin: e.Origin, CreatedAt: e.CreatedAt.Time}
		if e.ActorID.Valid {
			v.ActorID = &e.ActorID.UUID
		}
		events = append(events, v)
	}
	return result, events, err
}
func (s Service) Deliveries(ctx context.Context, a Actor, f ListFilter) (Page[Delivery], error) {
	if err := authorize(ctx, s.Store.Queries, a, "alert.read"); err != nil {
		return Page[Delivery]{}, err
	}
	ids, err := actorResources(ctx, s.Store.Queries, a)
	if err != nil {
		return Page[Delivery]{}, err
	}
	return s.deliveryPage(ctx, a, f, ids)
}

func (s Service) Delivery(ctx context.Context, a Actor, id uuid.UUID) (Delivery, []db.AlertDeliveryAttempt, error) {
	if err := authorize(ctx, s.Store.Queries, a, "alert.read"); err != nil {
		return Delivery{}, nil, err
	}
	row, err := s.Store.Queries.GetAlertDelivery(ctx, db.GetAlertDeliveryParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return Delivery{}, nil, ErrNotFound
	}
	ids, err := actorResources(ctx, s.Store.Queries, a)
	if err != nil || !containsAll(ids, row.ResourceIds) {
		return Delivery{}, nil, ErrDenied
	}
	item, err := deliveryDTO(row)
	if err != nil {
		return item, nil, err
	}
	attempts, err := s.Store.Queries.ListAlertDeliveryAttempts(ctx, db.ListAlertDeliveryAttemptsParams{EnterpriseID: a.EnterpriseID, DeliveryID: id})
	return item, attempts, err
}
func (s Service) Summary(ctx context.Context, a Actor) (Summary, error) {
	page, err := s.Instances(ctx, a, ListFilter{Limit: 500}, false)
	if err != nil {
		return Summary{}, err
	}
	result := Summary{CurrentTotal: page.Total}
	// Read every authorized result; the overview never substitutes one page for a total.
	for {
		for _, i := range page.Items {
			if i.Lifecycle == "firing" || i.Lifecycle == "recovering" {
				result.Active++
			}
			if !i.Acknowledged {
				result.Unacknowledged++
			}
			if result.LastEvaluatedAt == nil || i.LastEvaluatedAt.After(*result.LastEvaluatedAt) {
				value := i.LastEvaluatedAt
				result.LastEvaluatedAt = &value
			}
		}
		if page.NextCursor == nil {
			break
		}
		page, err = s.Instances(ctx, a, ListFilter{Limit: 500, Cursor: *page.NextCursor}, false)
		if err != nil {
			return result, err
		}
	}
	configs, err := s.Configurations(ctx, a, "policy")
	if err != nil {
		return result, err
	}
	result.Policies = len(configs)
	deliveries, err := s.Deliveries(ctx, a, ListFilter{Limit: 500})
	if err != nil {
		return result, err
	}
	for {
		for _, d := range deliveries.Items {
			if d.Status == "failed" || d.Status == "unknown" {
				result.FailedDeliveries++
			}
		}
		if deliveries.NextCursor == nil {
			break
		}
		deliveries, err = s.Deliveries(ctx, a, ListFilter{Limit: 500, Cursor: *deliveries.NextCursor})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}
func (s Service) Handle(ctx context.Context, a Actor, id uuid.UUID, input HandleInput, key string, origin string) (Instance, error) {
	if key == "" || !slices.Contains([]string{"acknowledge", "assign", "comment"}, input.Operation) || len(input.Comment) > 8000 || input.Operation == "comment" && strings.TrimSpace(input.Comment) == "" {
		return Instance{}, ErrInvalid
	}
	return retryAlertWrite(ctx, func() (Instance, error) {
		return postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", a.SubjectID.String(), "alert.handle."+input.Operation, key, []any{id, input}, 200, func(q *db.Queries) (Instance, error) {
			if origin != "model_agent" {
				origin = "admin_ui"
			}
			return handleWithQueries(ctx, q, a, id, input, origin)
		})
	})
}
