package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
)

// DeliveryGapWorker turns authenticated control evidence into the same alert
// instances, handling, history, routing, noise and notification lifecycle.
// It does not query a fabricated metric or require optional self-collection.
type DeliveryGapWorker struct {
	Service Service
	Logger  *slog.Logger
}

func (s Service) EnqueueCollectorDeliveryAlerts(ctx context.Context) error {
	return s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		rows, err := q.DueCollectorDeliveryAlerts(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			payload, _ := json.Marshal(map[string]any{"report_id": row.ID})
			id := uuid.New()
			if _, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: id, EnterpriseID: nullID(row.EnterpriseID), Queue: telemetry.CollectorDeliveryQueue, Payload: payload, MaxAttempts: 8, AvailableAt: timestamp(time.Now().UTC())}); err != nil {
				return err
			}
			// pending_alert stays durable until the alert worker commits its watermark.
		}
		return nil
	})
}

func (w DeliveryGapWorker) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.Service.EnqueueCollectorDeliveryAlerts(ctx); err != nil && ctx.Err() == nil {
			logger := w.Logger
			if logger == nil {
				logger = slog.Default()
			}
			logger.Warn("Collector delivery alert Outbox unavailable; durable intent retained")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w DeliveryGapWorker) Handle(ctx context.Context, task runtime.Task) error {
	var input struct {
		ReportID uuid.UUID `json:"report_id"`
	}
	if json.Unmarshal(task.Payload, &input) != nil || input.ReportID == uuid.Nil || !task.EnterpriseID.Valid {
		return ErrInvalid
	}
	return alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		owned, err := q.AlertRuntimeTaskOwned(ctx, db.AlertRuntimeTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if err != nil || !owned {
			return ErrConflict
		}
		row, err := q.LockCollectorDeliveryReport(ctx, db.LockCollectorDeliveryReportParams{EnterpriseID: task.EnterpriseID.UUID, ID: input.ReportID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.Sequence <= row.NotifiedSequence {
			return nil
		}
		var report, before telemetry.CollectorDeliveryReport
		if json.Unmarshal(row.Report, &report) != nil || report.Validate(time.Now().UTC()) != nil {
			return ErrInvalid
		}
		if row.NotifiedSequence > 0 && json.Unmarshal(row.NotifiedReport, &before) != nil {
			return ErrInvalid
		}
		if !report.HasNewFailure(before) {
			return q.MarkCollectorDeliveryNotified(ctx, db.MarkCollectorDeliveryNotifiedParams{EnterpriseID: row.EnterpriseID, ID: row.ID})
		}
		collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.CollectorID})
		if err != nil {
			return err
		}
		policyID := uuid.NewSHA1(collector.ID, []byte("collector-delivery-policy/v1"))
		ruleID := uuid.NewSHA1(policyID, []byte("observed_delivery_failures"))
		p := PolicySpec{Enabled: false, Description: "Managed control evidence. Historical data gaps remain until explicitly handled; exact targets and lost sample timestamps are unknown.", Resources: ResourceSelector{Mode: "explicit", IDs: []uuid.UUID{collector.ResourceID}, Types: []string{collector.ResourceType}}, Rules: []Rule{{ID: ruleID, Name: "Collector delivery data gap", Expression: "collector.delivery.observed_failures", QueryMode: "numeric", Operator: ">", Severity: "warning"}}, Mode: "independent", IntervalSeconds: 30, NoDataBehavior: "report", ErrorBehavior: "keep", ChannelIDs: []uuid.UUID{}, RouteIDs: []uuid.UUID{}, Labels: map[string]string{"argus.managed_by": "collector_delivery"}}
		channels, err := loadConfigurations(ctx, q, row.EnterpriseID, "channel")
		if err != nil {
			return err
		}
		for _, channel := range channels {
			var spec ChannelSpec
			if json.Unmarshal(channel.Spec, &spec) == nil && spec.Enabled && spec.DefaultForNewPolicies {
				p.ChannelIDs = append(p.ChannelIDs, channel.ID)
			}
		}
		policyRaw, _ := json.Marshal(p)
		policy, err := q.EnsureCollectorDeliveryPolicy(ctx, db.EnsureCollectorDeliveryPolicyParams{ID: policyID, EnterpriseID: row.EnterpriseID, Spec: policyRaw})
		if err != nil {
			return err
		}
		if err = q.BindCollectorDeliveryPolicy(ctx, db.BindCollectorDeliveryPolicyParams{PolicyID: policyID, CollectorID: collector.ID, EnterpriseID: row.EnterpriseID}); err != nil {
			return err
		}
		fingerprint := "collector_delivery:" + row.InstanceID.String()
		at := time.Now().UTC()
		state := InstanceState{Name: "Collector delivery data gap", PolicyName: policy.Name, RuleID: ruleID, Metric: "collector.delivery.observed_failures", Labels: map[string]string{"collector_id": collector.ID.String(), "reporter_id": row.InstanceID.String(), "attribution": report.Attribution, "retry_limit_seconds": strconv.FormatInt(report.RetryLimitSeconds, 10)}, Kind: "collector_delivery_gap", PolicySnapshot: policyRaw, NotificationState: "pending", NoiseReasons: []string{}, LastSlot: report.ObservedAt, DeliveryGap: &report}
		instance, err := q.GetActiveAlertFingerprint(ctx, db.GetActiveAlertFingerprintParams{EnterpriseID: row.EnterpriseID, PolicyID: policy.ID, Fingerprint: fingerprint})
		eventType := "first"
		if errors.Is(err, pgx.ErrNoRows) {
			raw, _ := json.Marshal(state)
			instance, err = q.CreateAlertInstance(ctx, db.CreateAlertInstanceParams{ID: uuid.New(), EnterpriseID: row.EnterpriseID, PolicyID: policy.ID, PolicyVersion: policy.Version, Generation: policy.Generation, Fingerprint: fingerprint, ResourceIds: []uuid.UUID{collector.ResourceID}, SourceIds: []uuid.UUID{}, State: raw, Lifecycle: "firing", Health: "error", Severity: "warning", StartedAt: timestamp(at), LastEvaluatedAt: timestamp(at)})
		} else if err == nil {
			var old InstanceState
			if json.Unmarshal(instance.State, &old) != nil {
				return ErrInvalid
			}
			state.LastNotificationAt = old.LastNotificationAt
			state.NotificationState = old.NotificationState
			raw, _ := json.Marshal(state)
			instance, err = q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: instance.ID, State: raw, Lifecycle: "firing", Health: "error", Severity: "warning", StartedAt: instance.StartedAt, LastEvaluatedAt: timestamp(at), Version: instance.Version})
			eventType = "repeat"
		}
		if err != nil {
			return err
		}
		event, err := appendLifecycleEvent(ctx, q, instance, eventType, at)
		if err != nil {
			return err
		}
		if err = planNotifications(ctx, q, policy, p, []db.AlertEvent{event}, at); err != nil {
			return err
		}
		return q.MarkCollectorDeliveryNotified(ctx, db.MarkCollectorDeliveryNotifiedParams{EnterpriseID: row.EnterpriseID, ID: row.ID})
	})
}
