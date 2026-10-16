package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Command struct {
	ExpectedHandlingVersion int64        `json:"expected_handling_version,omitempty"`
	Operation               string       `json:"operation"`
	ID                      uuid.UUID    `json:"id"`
	ExpectedVersion         int64        `json:"expected_version"`
	Handling                *HandleInput `json:"handling,omitempty"`
}
type commandPlan struct {
	Actor                Actor       `json:"actor"`
	Command              Command     `json:"command"`
	ResourceIDs          []uuid.UUID `json:"resource_ids"`
	ConfigurationVersion int64       `json:"configuration_version"`
}

func (s Service) PreviewCommand(ctx context.Context, a Actor, input Command, key string) (db.PendingAction, error) {
	if key == "" || input.ID == uuid.Nil {
		return db.PendingAction{}, ErrInvalid
	}
	if strings.HasPrefix(input.Operation, "event.") && input.Handling == nil {
		input.Handling = &HandleInput{Operation: strings.TrimPrefix(input.Operation, "event."), ExpectedHandlingVersion: input.ExpectedHandlingVersion}
	}
	plan, err := validateCommand(ctx, s.Store.Queries, a, input)
	if err != nil {
		return db.PendingAction{}, err
	}
	return retryAlertWrite(ctx, func() (db.PendingAction, error) {
		return s.Actions.PrepareForSubject(ctx, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, resource.PrepareActionInput{RunID: nullID(a.RunID), ActionType: "alert.command." + input.Operation, Title: input.Operation, Summary: "Apply confirmed alert operation", Risk: "write", ResourceType: "alert_operation", ResourceID: nullID(input.ID), ExpectedResourceVersion: pgtype.Int8{Int64: input.ExpectedVersion, Valid: input.ExpectedVersion > 0}, AuthorizationVersion: a.AuthorizationVersion, Preview: previewInvocation(a, key, map[string]any{"operation": input.Operation, "id": input.ID, "resource_ids": plan.ResourceIDs, "handling": input.Handling}), Diff: []map[string]string{{"kind": "change", "text": input.Operation}}, ImmutablePlan: plan, ResourceScopeSnapshot: plan, CommitHandler: "alert.command." + input.Operation + ".commit"}, key)
	})
}
func validateCommand(ctx context.Context, q *db.Queries, a Actor, input Command) (commandPlan, error) {
	plan := commandPlan{Actor: a, Command: input, ResourceIDs: []uuid.UUID{}}
	permission := "alert.notification.manage"
	if strings.HasPrefix(input.Operation, "event.") {
		permission = "alert.handle"
	}
	if authorize(ctx, q, a, permission) != nil {
		return plan, ErrDenied
	}
	switch input.Operation {
	case "notification.test":
		row, err := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: input.ID})
		if err != nil || row.Kind != "channel" {
			return plan, ErrNotFound
		}
		spec, _ := decodeSpec[ChannelSpec](row.Spec)
		if !spec.Enabled || row.Version != input.ExpectedVersion {
			return plan, ErrConflict
		}
		plan.ConfigurationVersion = row.Version
	case "notification.resend":
		row, err := q.GetAlertDelivery(ctx, db.GetAlertDeliveryParams{EnterpriseID: a.EnterpriseID, ID: input.ID})
		if err != nil {
			return plan, ErrNotFound
		}
		if row.Version != input.ExpectedVersion || row.Status != "failed" && row.Status != "unknown" {
			return plan, ErrConflict
		}
		plan.ResourceIDs = row.ResourceIds
		plan.ConfigurationVersion = row.ChannelVersion
		valid, _, e := notificationCurrent(ctx, q, row)
		if e != nil {
			return plan, e
		}
		if !valid {
			return plan, ErrConflict
		}
	case "event.acknowledge", "event.assign", "event.comment":
		row, err := q.GetAlertInstance(ctx, db.GetAlertInstanceParams{EnterpriseID: a.EnterpriseID, ID: input.ID})
		if err != nil {
			return plan, ErrNotFound
		}
		if row.HandlingVersion != input.ExpectedHandlingVersion || input.ExpectedHandlingVersion < 1 || input.Handling == nil {
			return plan, ErrConflict
		}
		plan.ResourceIDs = row.ResourceIds
	default:
		return plan, ErrInvalid
	}
	if len(plan.ResourceIDs) > 0 {
		allowed, err := actorResources(ctx, q, a)
		if err != nil || !containsAll(allowed, plan.ResourceIDs) {
			return plan, ErrDenied
		}
	}
	return plan, nil
}
func revalidateCommand(ctx context.Context, q *db.Queries, pending db.PendingAction, raw json.RawMessage) (commandPlan, error) {
	var saved commandPlan
	if json.Unmarshal(raw, &saved) != nil || saved.Actor.EnterpriseID != pending.EnterpriseID || saved.Actor.SubjectID != pending.CreatorSubjectID || saved.Actor.AuthorizationVersion != pending.AuthorizationVersion {
		return saved, ErrDenied
	}
	return validateCommand(ctx, q, saved.Actor, saved.Command)
}
func commitCommand(ctx context.Context, q *db.Queries, pending db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	plan, err := revalidateCommand(ctx, q, pending, raw)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	c := plan.Command
	version := int64(0)
	resultID, resultType := c.ID, "alert_instance"
	switch c.Operation {
	case "notification.test":
		message := Message{Subject: "[TEST] Argus alert notification", Text: "[TEST] Argus notification protocol test. This message does not represent a production alert.", EventType: "test", InstanceIDs: []uuid.UUID{}, PolicyIDs: []uuid.UUID{}, Labels: map[string]string{}, Test: true, Initiator: &plan.Actor}
		payload, _ := json.Marshal(message)
		d, e := q.CreateAlertDelivery(ctx, db.CreateAlertDeliveryParams{ID: uuid.NewSHA1(pending.ID, []byte("test")), EnterpriseID: pending.EnterpriseID, ChannelID: c.ID, ChannelVersion: plan.ConfigurationVersion, RouteVersion: 1, DedupKey: pending.ActionRef, GroupKey: pending.ActionRef, ResourceIds: []uuid.UUID{}, Payload: payload, Status: "pending", AvailableAt: timestamp(time.Now())})
		if e != nil {
			return resource.ActionCommitResult{}, e
		}
		if e = enqueueDelivery(ctx, q, d, time.Now()); e != nil {
			return resource.ActionCommitResult{}, e
		}
		version = d.Version
		resultID, resultType = d.ID, "alert_delivery"
	case "notification.resend":
		d, e := q.GetAlertDelivery(ctx, db.GetAlertDeliveryParams{EnterpriseID: pending.EnterpriseID, ID: c.ID})
		if e != nil {
			return resource.ActionCommitResult{}, e
		}
		d, e = q.CreateAlertDelivery(ctx, db.CreateAlertDeliveryParams{ID: uuid.NewSHA1(pending.ID, []byte("resend")), EnterpriseID: d.EnterpriseID, EventID: d.EventID, InstanceID: d.InstanceID, ChannelID: d.ChannelID, ChannelVersion: d.ChannelVersion, RouteVersion: d.RouteVersion, DedupKey: pending.ActionRef, GroupKey: pending.ActionRef, ResourceIds: d.ResourceIds, Payload: d.Payload, Status: "pending", AvailableAt: timestamp(time.Now())})
		if e != nil {
			return resource.ActionCommitResult{}, e
		}
		if e = q.SetAlertDeliveryResend(ctx, db.SetAlertDeliveryResendParams{EnterpriseID: d.EnterpriseID, ID: d.ID, ResendOfID: nullID(c.ID)}); e != nil {
			return resource.ActionCommitResult{}, e
		}
		if e = enqueueDelivery(ctx, q, d, time.Now()); e != nil {
			return resource.ActionCommitResult{}, e
		}
		version = d.Version
		resultID, resultType = d.ID, "alert_delivery"
	default:
		input := *c.Handling
		input.Operation = strings.TrimPrefix(c.Operation, "event.")
		input.ExpectedHandlingVersion = c.ExpectedHandlingVersion
		row, e := handleWithQueries(ctx, q, plan.Actor, c.ID, input, "model_agent")
		if e != nil {
			return resource.ActionCommitResult{}, e
		}
		version = row.Version
	}
	details, err := resource.PendingExecutionAuditDetails(ctx, q, pending, map[string]any{"operation": c.Operation, "resource_ids": plan.ResourceIDs, "resource_type": resultType, "resource_id": resultID})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if err = appendAudit(ctx, q, plan.Actor, "alert.command."+c.Operation, c.ID, details); err != nil {
		return resource.ActionCommitResult{}, err
	}
	return resource.ActionCommitResult{ResourceType: resultType, ResourceID: resultID, ResourceVersion: version, Summary: "Confirmed alert operation applied"}, nil
}
func handleWithQueries(ctx context.Context, q *db.Queries, a Actor, id uuid.UUID, input HandleInput, origin string) (Instance, error) {
	if authorize(ctx, q, a, "alert.handle") != nil {
		return Instance{}, ErrDenied
	}
	row, err := q.LockAlertInstance(ctx, db.LockAlertInstanceParams{EnterpriseID: a.EnterpriseID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	if row.HandlingVersion != input.ExpectedHandlingVersion || input.ExpectedHandlingVersion < 1 {
		return Instance{}, ErrConflict
	}
	allowed, err := actorResources(ctx, q, a)
	if err != nil || !containsAll(allowed, row.ResourceIds) {
		return Instance{}, ErrDenied
	}
	ack := row.Acknowledged
	owner := row.OwnerUserID
	switch input.Operation {
	case "acknowledge":
		ack = true
	case "assign":
		owner = uuid.NullUUID{}
		if input.OwnerID != nil {
			user, e := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: a.EnterpriseID, ID: *input.OwnerID})
			if e != nil || user.Status != "active" {
				return Instance{}, ErrInvalid
			}
			other := Actor{EnterpriseID: a.EnterpriseID, SubjectID: user.ID, SubjectType: "user", AuthorizationVersion: user.AuthorizationVersion}
			if authorize(ctx, q, other, "alert.read") != nil {
				return Instance{}, ErrDenied
			}
			otherIDs, e := actorResources(ctx, q, other)
			if e != nil || !containsAll(otherIDs, row.ResourceIds) {
				return Instance{}, ErrDenied
			}
			owner = nullID(user.ID)
		}
	case "comment":
		if len(input.Comment) == 0 || len(input.Comment) > 8000 {
			return Instance{}, ErrInvalid
		}
	default:
		return Instance{}, ErrInvalid
	}
	updated, err := q.HandleAlertInstance(ctx, db.HandleAlertInstanceParams{EnterpriseID: a.EnterpriseID, ID: id, Acknowledged: ack, OwnerUserID: owner, ExpectedHandlingVersion: row.HandlingVersion})
	if err != nil {
		return Instance{}, err
	}
	payload, _ := json.Marshal(input)
	if _, err = q.AddAlertEvent(ctx, db.AddAlertEventParams{ID: uuid.New(), EnterpriseID: a.EnterpriseID, InstanceID: id, PolicyID: row.PolicyID, EventType: input.Operation, ResourceIds: row.ResourceIds, Payload: payload, ActorID: nullID(a.SubjectID), Origin: origin, CreatedAt: timestamp(time.Now())}); err != nil {
		return Instance{}, err
	}
	if err = appendAudit(ctx, q, a, "alert.handle."+input.Operation, id, map[string]any{"operation": input.Operation, "origin": origin, "resource_ids": row.ResourceIds}); err != nil {
		return Instance{}, err
	}
	return instanceDTO(updated)
}
