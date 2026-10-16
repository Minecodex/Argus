package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type NotificationWorker struct {
	Service   Service
	Transport Transport
}
type deliveryTask struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
}
type deliveryBatch struct {
	Persistent db.AlertDeliveryBatch
	Channel    db.AlertConfiguration
	Spec       ChannelSpec
	Deliveries []db.AlertDelivery
	Message    Message
}

func (w NotificationWorker) Handle(ctx context.Context, task runtime.Task) error {
	var input deliveryTask
	if json.Unmarshal(task.Payload, &input) != nil || !task.EnterpriseID.Valid {
		return ErrInvalid
	}
	batch, err := w.claim(ctx, task, input.DeliveryID)
	if err != nil {
		return err
	}
	if len(batch.Deliveries) == 0 {
		return nil
	}
	lease, err := w.Service.Secrets.IssueLease(ctx, batch.Channel.CreatedBy.String(), task.EnterpriseID.UUID, secret.LeaseRequest{CredentialID: batch.Spec.CredentialID, OperationRef: input.DeliveryID.String(), TargetResourceType: "alert_channel", TargetResourceID: batch.Channel.ID, RecipientType: "notification_worker", RecipientID: task.ID.String(), Protocol: "notification", TTL: time.Minute, RuntimeTaskID: task.ID, LeaseOwner: task.LeaseOwner.String, FenceToken: task.FenceToken})
	defer clear(lease.Value)
	if err != nil && !errors.Is(err, secret.ErrCredentialUnavailable) && !errors.Is(err, secret.ErrInvalidLease) {
		// The batch is still prepared. Let the fenced runtime recovery retry a
		// database/broker failure without turning it into a permanent rejection.
		return notificationCredentialError("issue", task, batch, err)
	}
	result := SendResult{ErrorCode: "NOTIFICATION_CREDENTIAL_UNAVAILABLE"}
	if err == nil {
		var credentials NotificationCredential
		if json.Unmarshal(lease.Value, &credentials) == nil {
			// Consume before any external side effect. Recovery of a sending
			// batch is Unknown, not an automatic second transmission.
			consumeErr := w.Service.Secrets.ConsumeLease(ctx, task.EnterpriseID.UUID, lease.Lease.ID)
			if consumeErr != nil && !errors.Is(consumeErr, secret.ErrInvalidLease) {
				return notificationCredentialError("consume", task, batch, consumeErr)
			}
			if consumeErr == nil {
				if valid, reason := w.preflight(ctx, task, batch); valid {
					if err := w.beginSend(ctx, task, batch); err != nil {
						return err
					}
					result = w.Transport.Send(ctx, batch.Spec, credentials, batch.Message)
				} else {
					result = SendResult{Canceled: true, ErrorCode: reason}
				}
			}
		}
	}
	return w.finish(ctx, task, batch, result)
}
func (w NotificationWorker) claim(ctx context.Context, task runtime.Task, id uuid.UUID) (deliveryBatch, error) {
	batch := deliveryBatch{}
	err := alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		batch = deliveryBatch{}
		owned, err := q.AlertNotificationTaskOwned(ctx, db.AlertNotificationTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken, Column4: id.String(), EnterpriseID: task.EnterpriseID})
		if err != nil || !owned {
			return ErrConflict
		}
		first, err := q.GetAlertDelivery(ctx, db.GetAlertDeliveryParams{EnterpriseID: task.EnterpriseID.UUID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if first.Status == "sending" {
			return w.recoverBatch(ctx, q, task, first)
		}
		if first.Status != "pending" {
			return nil
		}
		group, err := q.AlertGroupNextSend(ctx, db.AlertGroupNextSendParams{EnterpriseID: first.EnterpriseID, ChannelID: first.ChannelID, GroupKey: first.GroupKey, NextSendAt: first.AvailableAt})
		if err != nil {
			return err
		}
		if group.NextSendAt.Time.After(time.Now()) {
			updated, e := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: first.EnterpriseID, ID: first.ID, Status: "pending", Attempt: first.Attempt, LastError: "NOTIFICATION_GROUP_WAIT", LastAttemptAt: first.LastAttemptAt, AvailableAt: group.NextSendAt, Version: first.Version})
			if e != nil {
				return e
			}
			return enqueueDelivery(ctx, q, updated, group.NextSendAt.Time)
		}
		channel, err := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: first.EnterpriseID, ID: first.ChannelID})
		if err != nil {
			return cancelDelivery(ctx, q, first, "NOTIFICATION_CHANNEL_UNAVAILABLE")
		}
		spec, err := decodeSpec[ChannelSpec](channel.Spec)
		if err != nil || !spec.Enabled || channel.Version != first.ChannelVersion {
			return cancelDelivery(ctx, q, first, "NOTIFICATION_CHANNEL_CHANGED")
		}
		valid, reason, err := notificationCurrent(ctx, q, first)
		if err != nil {
			return err
		}
		if !valid {
			return cancelDelivery(ctx, q, first, reason)
		}
		rows, err := q.LockAlertDeliveryGroup(ctx, db.LockAlertDeliveryGroupParams{EnterpriseID: first.EnterpriseID, ChannelID: first.ChannelID, GroupKey: first.GroupKey})
		if err != nil {
			return err
		}
		found := false
		for _, d := range rows {
			if d.ID == id {
				found = true
			}
		}
		if !found {
			return nil
		}
		spacing := time.Second
		if spec.Provider == "dingtalk" || spec.Provider == "wecom" {
			spacing = 3 * time.Second
		}
		if _, e := q.ClaimAlertChannelRate(ctx, db.ClaimAlertChannelRateParams{EnterpriseID: first.EnterpriseID, ChannelID: first.ChannelID, NotBefore: timestamp(time.Now().Add(spacing))}); errors.Is(e, pgx.ErrNoRows) {
			next := time.Now().Add(spacing)
			updated, e := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: first.EnterpriseID, ID: first.ID, Status: "pending", Attempt: first.Attempt, LastError: "NOTIFICATION_RATE_WAIT", LastAttemptAt: first.LastAttemptAt, AvailableAt: timestamp(next), Version: first.Version})
			if e != nil {
				return e
			}
			return enqueueDelivery(ctx, q, updated, next)
		} else if e != nil {
			return e
		}
		for index, row := range rows {
			if row.ID == id {
				rows[0], rows[index] = rows[index], rows[0]
				break
			}
		}
		batch.Channel = channel
		batch.Spec = spec
		messages := []string{}
		resources := []uuid.UUID{}
		for _, d := range rows {
			if d.ChannelVersion != channel.Version {
				if err = cancelDelivery(ctx, q, d, "NOTIFICATION_CHANNEL_CHANGED"); err != nil {
					return err
				}
				continue
			}
			valid, reason, e := notificationCurrent(ctx, q, d)
			if e != nil {
				return e
			}
			if !valid {
				if e = cancelDelivery(ctx, q, d, reason); e != nil {
					return e
				}
				continue
			}
			message := Message{}
			if json.Unmarshal(d.Payload, &message) != nil {
				return ErrInvalid
			}
			if len(strings.Join(messages, "\n\n"))+len(message.Text) > 48<<10 && len(batch.Deliveries) > 0 {
				break
			}
			updated, e := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID, Status: "sending", Attempt: d.Attempt + 1, LastError: "", LastAttemptAt: timestamp(time.Now()), AvailableAt: d.AvailableAt, Version: d.Version})
			if e != nil {
				return e
			}
			batch.Deliveries = append(batch.Deliveries, updated)
			if batch.Message.Subject == "" {
				batch.Message = message
			} else {
				batch.Message.InstanceIDs = append(batch.Message.InstanceIDs, message.InstanceIDs...)
				batch.Message.PolicyIDs = append(batch.Message.PolicyIDs, message.PolicyIDs...)
			}
			messages = append(messages, message.Text)
			resources = append(resources, d.ResourceIds...)
		}
		if len(batch.Deliveries) > 1 {
			batch.Message.Subject = fmt.Sprintf("[%d alerts] %s", len(batch.Deliveries), batch.Message.Subject)
		}
		batch.Message.Text = strings.Join(messages, "\n\n")
		_ = resources
		if len(batch.Deliveries) > 0 {
			batch.Message.BatchID = uuid.New()
			payload, _ := json.Marshal(batch.Message)
			batch.Persistent, err = q.CreateAlertDeliveryBatch(ctx, db.CreateAlertDeliveryBatchParams{ID: batch.Message.BatchID, EnterpriseID: first.EnterpriseID, ChannelID: first.ChannelID, ChannelVersion: channel.Version, GroupKey: first.GroupKey, OwnerTaskID: task.ID, Fence: task.FenceToken, Payload: payload})
			if err != nil {
				return err
			}
			for _, d := range batch.Deliveries {
				if err = q.AttachAlertDeliveryBatch(ctx, db.AttachAlertDeliveryBatchParams{EnterpriseID: d.EnterpriseID, ID: d.ID, BatchID: nullID(batch.Persistent.ID)}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return batch, err
}
func notificationCurrent(ctx context.Context, q *db.Queries, d db.AlertDelivery) (bool, string, error) {
	var message Message
	if json.Unmarshal(d.Payload, &message) != nil {
		return false, "NOTIFICATION_PAYLOAD_INVALID", ErrInvalid
	}
	for id, version := range message.RoutingSnapshot {
		uid, e := uuid.Parse(id)
		if e != nil {
			return false, "NOTIFICATION_ROUTE_INVALID", nil
		}
		route, e := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: d.EnterpriseID, ID: uid})
		if e != nil || route.Version != version {
			return false, "NOTIFICATION_ROUTE_CHANGED", nil
		}
		spec, e := decodeSpec[RouteSpec](route.Spec)
		if e != nil || !spec.Enabled {
			return false, "NOTIFICATION_ROUTE_DISABLED", nil
		}
	}
	if message.Test {
		if message.Initiator == nil || authorize(ctx, q, *message.Initiator, "alert.notification.manage") != nil {
			return false, "NOTIFICATION_INITIATOR_REVOKED", nil
		}
		return true, "", nil
	}
	eval, err := evaluator(ctx, q, d.EnterpriseID)
	if err != nil {
		return false, "NOTIFICATION_EVALUATOR_UNAVAILABLE", nil
	}
	ids, err := actorResources(ctx, q, eval)
	if err != nil {
		return false, "NOTIFICATION_SCOPE_DENIED", err
	}
	if !containsAll(ids, d.ResourceIds) {
		return false, "NOTIFICATION_SCOPE_DENIED", nil
	}
	if !d.InstanceID.Valid {
		return false, "NOTIFICATION_INSTANCE_UNAVAILABLE", nil
	}
	row, err := q.GetAlertInstance(ctx, db.GetAlertInstanceParams{EnterpriseID: d.EnterpriseID, ID: d.InstanceID.UUID})
	if err != nil {
		return false, "NOTIFICATION_INSTANCE_UNAVAILABLE", nil
	}
	policy, err := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: d.EnterpriseID, ID: row.PolicyID})
	if err != nil {
		return false, "NOTIFICATION_POLICY_UNAVAILABLE", nil
	}
	p, _ := decodeSpec[PolicySpec](policy.Spec)
	if !p.Enabled || row.Generation != policy.Generation {
		return false, "NOTIFICATION_POLICY_CHANGED", nil
	}
	if message.EventType == "recovery" {
		if row.Lifecycle != "resolved" {
			return false, "NOTIFICATION_RECOVERY_CHANGED", nil
		}
	} else if row.Lifecycle != "firing" && row.Lifecycle != "recovering" {
		return false, "NOTIFICATION_EVENT_RESOLVED", nil
	}
	silences, err := loadConfigurations(ctx, q, d.EnterpriseID, "silence")
	if err != nil {
		return false, "", err
	}
	inhibitions, err := loadConfigurations(ctx, q, d.EnterpriseID, "inhibition")
	if err != nil {
		return false, "", err
	}
	rows, err := q.ListAlertInstances(ctx, d.EnterpriseID)
	if err != nil {
		return false, "", err
	}
	all := []Instance{}
	for _, r := range rows {
		v, e := instanceDTO(r)
		if e != nil {
			return false, "", e
		}
		all = append(all, v)
	}
	instance, err := instanceDTO(row)
	if err != nil {
		return false, "", err
	}
	state, _ := noiseDecision(instance, all, silences, inhibitions, time.Now().UTC())
	if state != "pending" {
		return false, "NOTIFICATION_" + strings.ToUpper(state), nil
	}
	return true, "", nil
}
func cancelDelivery(ctx context.Context, q *db.Queries, row db.AlertDelivery, reason string) error {
	_, err := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: "canceled", Attempt: row.Attempt, LastError: reason, LastAttemptAt: row.LastAttemptAt, AvailableAt: row.AvailableAt, Version: row.Version})
	return err
}
func (w NotificationWorker) markUnknown(ctx context.Context, q *db.Queries, d db.AlertDelivery) error {
	_, err := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID, Status: "unknown", Attempt: d.Attempt, LastError: "NOTIFICATION_RESULT_UNKNOWN", LastAttemptAt: d.LastAttemptAt, AvailableAt: d.AvailableAt, Version: d.Version})
	if err != nil {
		return err
	}
	return q.AddAlertDeliveryAttempt(ctx, db.AddAlertDeliveryAttemptParams{ID: uuid.New(), EnterpriseID: d.EnterpriseID, DeliveryID: d.ID, Attempt: d.Attempt, Status: "unknown", ErrorCode: "NOTIFICATION_RESULT_UNKNOWN", StartedAt: d.LastAttemptAt, FinishedAt: timestamp(time.Now())})
}
func (w NotificationWorker) finish(ctx context.Context, task runtime.Task, batch deliveryBatch, result SendResult) error {
	return alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		owned, err := q.AlertRuntimeTaskOwned(ctx, db.AlertRuntimeTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if err != nil || !owned {
			return ErrConflict
		}
		current, err := q.GetAlertDeliveryBatch(ctx, db.GetAlertDeliveryBatchParams{EnterpriseID: batch.Persistent.EnterpriseID, ID: batch.Persistent.ID})
		if err != nil {
			return err
		}
		if current.OwnerTaskID != task.ID || current.Fence != task.FenceToken || current.Status != "sending" && current.Status != "prepared" {
			return ErrConflict
		}
		batchStatus := "failed"
		if result.Accepted {
			batchStatus = "accepted"
		}
		if result.Unknown {
			batchStatus = "unknown"
		}
		if result.Canceled {
			batchStatus = "canceled"
		}
		for _, d := range batch.Deliveries {
			status := "failed"
			if result.Accepted {
				status = "accepted"
			}
			if result.Unknown {
				status = "unknown"
			}
			if result.Canceled {
				status = "canceled"
			}
			next := time.Now().UTC()
			if result.Retryable && !result.Unknown && d.Attempt < 5 && time.Now().Before(d.RetryUntil.Time) {
				status = "pending"
				delay := time.Duration(1<<d.Attempt) * time.Second
				if result.RetryAfter > delay {
					delay = result.RetryAfter
				}
				next = next.Add(delay)
			}
			updated, e := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID, Status: status, Attempt: d.Attempt, LastError: result.ErrorCode, LastAttemptAt: d.LastAttemptAt, AvailableAt: timestamp(next), Version: d.Version})
			if e != nil {
				return e
			}
			attemptStatus := status
			if status == "pending" {
				attemptStatus = "failed"
			}
			if e = q.AddAlertDeliveryAttempt(ctx, db.AddAlertDeliveryAttemptParams{ID: uuid.New(), EnterpriseID: d.EnterpriseID, DeliveryID: d.ID, Attempt: d.Attempt, Status: attemptStatus, ResultCode: result.Code, ErrorCode: result.ErrorCode, StartedAt: d.LastAttemptAt, FinishedAt: timestamp(time.Now())}); e != nil {
				return e
			}
			if status == "pending" {
				if e = enqueueDelivery(ctx, q, updated, next); e != nil {
					return e
				}
			}
			if d.InstanceID.Valid {
				row, e := q.LockAlertInstance(ctx, db.LockAlertInstanceParams{EnterpriseID: d.EnterpriseID, ID: d.InstanceID.UUID})
				if e != nil {
					return e
				}
				var state InstanceState
				if json.Unmarshal(row.State, &state) != nil {
					return ErrInvalid
				}
				state.NotificationState = status
				deliveries, e := q.ListAlertDeliveries(ctx, d.EnterpriseID)
				if e != nil {
					return e
				}
				for _, other := range deliveries {
					if other.EventID == d.EventID && other.InstanceID == d.InstanceID && deliveryRank(other.Status) > deliveryRank(state.NotificationState) {
						state.NotificationState = other.Status
					}
				}
				raw, _ := json.Marshal(state)
				if _, e = q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.ID, State: raw, Lifecycle: row.Lifecycle, Health: row.Health, Severity: row.Severity, StartedAt: row.StartedAt, ResolvedAt: row.ResolvedAt, LastEvaluatedAt: row.LastEvaluatedAt, Version: row.Version}); e != nil {
					return e
				}
			}
		}
		if err = q.SetAlertBatchStatus(ctx, db.SetAlertBatchStatusParams{EnterpriseID: current.EnterpriseID, ID: current.ID, Status: batchStatus}); err != nil {
			return err
		}
		if result.Accepted {
			grouping, e := groupingConfiguration(ctx, q, current.EnterpriseID)
			if e != nil {
				return e
			}
			return q.SetAlertGroupSent(ctx, db.SetAlertGroupSentParams{EnterpriseID: current.EnterpriseID, ChannelID: current.ChannelID, GroupKey: current.GroupKey, NextSendAt: timestamp(time.Now().Add(time.Duration(grouping.IntervalSeconds) * time.Second))})
		}
		return nil
	})
}

func deliveryRank(status string) int {
	switch status {
	case "unknown":
		return 7
	case "failed":
		return 6
	case "sending", "pending":
		return 5
	case "canceled":
		return 4
	case "muted", "inhibited":
		return 3
	case "accepted":
		return 1
	default:
		return 0
	}
}

func (w NotificationWorker) preflight(ctx context.Context, task runtime.Task, batch deliveryBatch) (bool, string) {
	owned, e := w.Service.Store.Queries.AlertRuntimeTaskOwned(ctx, db.AlertRuntimeTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
	if e != nil || !owned {
		return false, "NOTIFICATION_FENCE_LOST"
	}
	row, e := w.Service.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: batch.Channel.EnterpriseID, ID: batch.Channel.ID})
	if e != nil || row.Version != batch.Channel.Version {
		return false, "NOTIFICATION_CHANNEL_CHANGED"
	}
	for _, d := range batch.Deliveries {
		valid, reason, e := notificationCurrent(ctx, w.Service.Store.Queries, d)
		if e != nil {
			return false, "NOTIFICATION_VALIDATION_FAILED"
		}
		if !valid {
			return false, reason
		}
	}
	return true, ""
}
