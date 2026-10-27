package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const EvaluationQueue = "alert-evaluation"
const NotificationQueue = "alert-notification"

type evaluationTask struct {
	PolicyID uuid.UUID `json:"policy_id"`
	Version  int64     `json:"version"`
	Slot     time.Time `json:"slot"`
}
type Worker struct {
	Service Service
	Logger  *slog.Logger
}

func (w Worker) Schedule(ctx context.Context) error {
	return alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		rows, err := q.DueAlertPolicies(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			p, err := decodeSpec[PolicySpec](row.Spec)
			if err != nil {
				return err
			}
			slot := time.Now().UTC().Truncate(time.Duration(p.IntervalSeconds) * time.Second)
			payload, _ := json.Marshal(evaluationTask{PolicyID: row.ID, Version: row.Version, Slot: slot})
			id := uuid.NewSHA1(row.ID, []byte(fmt.Sprintf("%s:%d", slot.Format(time.RFC3339Nano), row.Version)))
			if _, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: id, EnterpriseID: nullID(row.EnterpriseID), Queue: EvaluationQueue, Payload: payload, MaxAttempts: 5, AvailableAt: timestamp(time.Now())}); err != nil {
				return err
			}
			if err = q.SetAlertNextEvaluation(ctx, db.SetAlertNextEvaluationParams{EnterpriseID: row.EnterpriseID, ID: row.ID, NextEvaluationAt: timestamp(slot.Add(time.Duration(p.IntervalSeconds) * time.Second))}); err != nil {
				return err
			}
		}
		return nil
	})
}
func (w Worker) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := w.Schedule(ctx); err != nil && ctx.Err() == nil && w.Logger != nil {
			w.Logger.Error("schedule alert evaluation", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w Worker) Handle(ctx context.Context, task runtime.Task) error {
	var input evaluationTask
	if json.Unmarshal(task.Payload, &input) != nil || !task.EnterpriseID.Valid {
		return ErrInvalid
	}
	row, err := w.Service.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: task.EnterpriseID.UUID, ID: input.PolicyID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.Version != input.Version || row.Kind != "policy" {
		return nil
	}
	p, err := decodeSpec[PolicySpec](row.Spec)
	if err != nil {
		return err
	}
	if !p.Enabled {
		return nil
	}
	result := Evaluation{Observations: []Observation{}, Outcomes: []EvaluationOutcome{}, Sources: []db.AlertMetricSourcesRow{}, ResourceIDs: []uuid.UUID{}, Health: "error", ErrorCode: "ALERT_EVALUATOR_UNAVAILABLE", Slot: input.Slot}
	eval, e := evaluator(ctx, w.Service.Store.Queries, row.EnterpriseID)
	result.Evaluator = eval
	if p.Resources.Mode == "explicit" {
		result.ResourceIDs = p.Resources.IDs
	}
	if e == nil {
		ids, e := selectedResources(ctx, w.Service.Store.Queries, eval, p.Resources)
		if e == nil {
			result, e = w.Service.evaluate(ctx, eval, p, ids, input.Slot)
		}
		if e != nil {
			result = Evaluation{Observations: []Observation{}, Health: "authorization_denied", ErrorCode: "ALERT_SCOPE_DENIED", Evaluator: eval, Slot: input.Slot}
		}
	}
	return w.persistEvaluation(ctx, task, row, p, result)
}
func (w Worker) persistEvaluation(ctx context.Context, task runtime.Task, policy db.AlertConfiguration, p PolicySpec, result Evaluation) error {
	return alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		owned, err := q.AlertRuntimeTaskOwned(ctx, db.AlertRuntimeTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if err != nil || !owned {
			return ErrConflict
		}
		current, err := q.LockAlertConfiguration(ctx, db.LockAlertConfigurationParams{EnterpriseID: policy.EnterpriseID, ID: policy.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.Version != policy.Version {
			return nil
		}
		// PostgreSQL stores this fence at microsecond precision. A nanosecond
		// remainder must not let the same persisted slot run twice.
		if current.LastEvaluatedAt.Valid && !result.Slot.Truncate(time.Microsecond).After(current.LastEvaluatedAt.Time) {
			return nil
		}
		if result.Evaluator.SubjectID != uuid.Nil && (result.Health == "normal" || result.Health == "no_data" || result.Health == "not_evaluated" || len(result.Observations) > 0) {
			account, e := evaluator(ctx, q, policy.EnterpriseID)
			if e != nil || account.AuthorizationVersion != result.Evaluator.AuthorizationVersion || account.SubjectID != result.Evaluator.SubjectID {
				return ErrDenied
			}
			sources, scope, e := metricSources(ctx, q, account, result.ResourceIDs, p.Resources.SourceIDs, p.Resources.Types)
			if e != nil || !sourcesEqual(sources, result.Sources) || scope.Signature != result.ScopeSignature {
				return ErrDenied
			}
			if result.SourcesCaptured {
				stopped, err := metricScopeExplicitlyStopped(ctx, q, account, scope)
				if err != nil {
					return err
				}
				if stopped != (result.Health == "not_evaluated") {
					return ErrDenied
				}
			}
			ids, e := selectedResources(ctx, q, account, p.Resources)
			if e != nil || !slices.Equal(ids, result.ResourceIDs) {
				return ErrDenied
			}
		}
		persistedResult, _ := json.Marshal(result)
		snapshot, _ := json.Marshal(map[string]any{"resources": result.ResourceRefs, "sources": result.Sources, "scope_signature": result.ScopeSignature})
		if err = q.AddAlertEvaluation(ctx, db.AddAlertEvaluationParams{ID: uuid.New(), EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID, PolicyVersion: policy.Version, Slot: timestamp(result.Slot), EvaluatorID: nullID(result.Evaluator.SubjectID), AuthorizationVersion: result.Evaluator.AuthorizationVersion, Scope: snapshot, Health: result.Health, ErrorCode: result.ErrorCode, Fence: task.FenceToken, Result: persistedResult}); err != nil {
			return err
		}
		old, err := q.ListPolicyAlertInstances(ctx, db.ListPolicyAlertInstancesParams{EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID})
		if err != nil {
			return err
		}
		for _, row := range old {
			if row.Generation != policy.Generation {
				if err := closeInstance(ctx, q, row, "configuration_changed", result.Slot); err != nil {
					return err
				}
			}
		}
		seen := map[string]bool{}
		generated := []db.AlertEvent{}
		// Combined observations contain every required rule for one identity.
		// Incomplete members elsewhere cannot block a complete resource/source group.
		if result.Health == "normal" || len(result.Observations) > 0 {
			for _, obs := range result.Observations {
				fp := fingerprint(policy.ID, policy.Generation, obs.Key)
				seen[fp] = true
				row, e := q.GetActiveAlertFingerprint(ctx, db.GetActiveAlertFingerprintParams{EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID, Fingerprint: fp})
				exists := e == nil
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
				state := InstanceState{Name: obs.Rule.Name, PolicyName: policy.Name, Labels: obs.Labels, PolicySnapshot: policy.Spec, NotificationState: "pending", NoiseReasons: []string{}, Kind: "condition"}
				life := "normal"
				if exists {
					if json.Unmarshal(row.State, &state) != nil {
						return ErrInvalid
					}
					life = row.Lifecycle
				}
				life, state, eventType := advance(life, state, obs, p, result.Slot)
				state.Name = obs.Rule.Name
				state.PolicyName = policy.Name
				if !exists || !row.StartedAt.Valid {
					state.PolicySnapshot = policy.Spec
				}
				state.EvaluatorID = result.Evaluator.SubjectID
				state.AuthorizationVersion = result.Evaluator.AuthorizationVersion
				state.LatestSourceSnapshot, _ = json.Marshal(result.Sources)
				if !exists || !row.StartedAt.Valid {
					state.SourceSnapshot = state.LatestSourceSnapshot
				}
				if eventType == "" && exists && row.Severity != obs.Severity && (life == "firing" || life == "recovering") {
					eventType = "severity"
				}
				raw, _ := json.Marshal(state)
				start := row.StartedAt
				resolved := row.ResolvedAt
				if eventType == "first" {
					start = timestamp(result.Slot)
				}
				if eventType == "recovery" {
					resolved = timestamp(result.Slot)
				}
				if exists {
					row, e = q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: policy.EnterpriseID, ID: row.ID, State: raw, Lifecycle: life, Health: "normal", Severity: obs.Severity, StartedAt: start, ResolvedAt: resolved, LastEvaluatedAt: timestamp(result.Slot), Version: row.Version, PolicyVersion: pgtype.Int8{Int64: policy.Version, Valid: !row.StartedAt.Valid}})
				} else {
					row, e = q.CreateAlertInstance(ctx, db.CreateAlertInstanceParams{ID: uuid.New(), EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID, PolicyVersion: policy.Version, Generation: policy.Generation, Fingerprint: fp, ResourceIds: obs.ResourceIDs, SourceIds: obs.SourceIDs, State: raw, Lifecycle: life, Health: "normal", Severity: obs.Severity, StartedAt: start, LastEvaluatedAt: timestamp(result.Slot)})
				}
				if e != nil {
					return e
				}
				if eventType != "" {
					event, e := appendLifecycleEvent(ctx, q, row, eventType, result.Slot)
					if e != nil {
						return e
					}
					generated = append(generated, event)
				}
			}
		}
		// Missing members and failed evaluations keep existing firing episodes;
		// they never turn an unavailable query into a fabricated recovery.
		for _, row := range old {
			if row.Generation != policy.Generation || seen[row.Fingerprint] {
				continue
			}
			var state InstanceState
			if json.Unmarshal(row.State, &state) != nil {
				return ErrInvalid
			}
			if state.Kind == "health" {
				continue
			}
			closure, e := "", error(nil)
			if result.Health != "not_evaluated" {
				closure, e = missingEpisodeClosure(ctx, q, row, result)
			}
			if e != nil {
				return e
			}
			if closure != "" {
				if e = closeInstance(ctx, q, row, closure, result.Slot); e != nil {
					return e
				}
				continue
			}
			state = evaluationFailure(state)
			state.LastSlot = result.Slot
			raw, _ := json.Marshal(state)
			health := result.Health
			if p.Mode == "independent" {
				for _, outcome := range result.Outcomes {
					if outcome.RuleID == state.RuleID && outcome.Health != "normal" && overlappingIDs(row.ResourceIds, outcome.ResourceIDs) {
						health = outcome.Health
						break
					}
				}
			}
			if health == "normal" {
				health = "no_data"
			}
			if result.Evaluator.SubjectID != uuid.Nil && !containsAll(result.ResourceIDs, row.ResourceIds) {
				health = "authorization_denied"
			}
			if _, err = q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.ID, State: raw, Lifecycle: row.Lifecycle, Health: health, Severity: row.Severity, StartedAt: row.StartedAt, ResolvedAt: row.ResolvedAt, LastEvaluatedAt: timestamp(result.Slot), Version: row.Version}); err != nil {
				return err
			}
		}
		report := result.Health == "no_data" && p.NoDataBehavior == "report" || result.Health != "normal" && result.Health != "no_data" && result.Health != "not_evaluated" && p.ErrorBehavior == "report"
		if report && len(result.ResourceIDs) > 0 {
			event, e := w.healthEpisode(ctx, q, policy, p, result)
			if e != nil {
				return e
			}
			if event.ID != uuid.Nil {
				generated = append(generated, event)
			}
		} else if result.Health == "normal" {
			rows, e := q.ListPolicyAlertInstances(ctx, db.ListPolicyAlertInstancesParams{EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID})
			if e != nil {
				return e
			}
			for _, row := range rows {
				var st InstanceState
				_ = json.Unmarshal(row.State, &st)
				if st.Kind == "health" {
					if row.Lifecycle == "firing" || row.Lifecycle == "recovering" {
						var st InstanceState
						_ = json.Unmarshal(row.State, &st)
						st.ClosureReason = "query_recovered"
						raw, _ := json.Marshal(st)
						updated, e := q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.ID, State: raw, Lifecycle: "resolved", Health: "normal", Severity: row.Severity, StartedAt: row.StartedAt, ResolvedAt: timestamp(result.Slot), LastEvaluatedAt: timestamp(result.Slot), Version: row.Version})
						if e != nil {
							return e
						}
						event, e := appendLifecycleEvent(ctx, q, updated, "recovery", result.Slot)
						if e != nil {
							return e
						}
						generated = append(generated, event)
					}
				}
			}
		}
		success := pgtype.Timestamptz{}
		if result.Health == "normal" {
			success = timestamp(result.Slot)
		}
		if err = q.SetAlertEvaluationHealth(ctx, db.SetAlertEvaluationHealthParams{EnterpriseID: policy.EnterpriseID, ID: policy.ID, EvaluationHealth: result.Health, EvaluationError: result.ErrorCode, LastEvaluatedAt: timestamp(result.Slot), LastSuccessfulAt: success, Version: policy.Version}); err != nil {
			return err
		}
		return planNotifications(ctx, q, policy, p, generated, result.Slot)
	})
}
func appendLifecycleEvent(ctx context.Context, q *db.Queries, row db.AlertInstance, kind string, at time.Time) (db.AlertEvent, error) {
	instance, err := instanceDTO(row)
	if err != nil {
		return db.AlertEvent{}, err
	}
	payload, _ := json.Marshal(instance)
	return q.AddAlertEvent(ctx, db.AddAlertEventParams{ID: uuid.New(), EnterpriseID: row.EnterpriseID, InstanceID: row.ID, PolicyID: row.PolicyID, EventType: kind, ResourceIds: row.ResourceIds, Payload: payload, Origin: "evaluator", CreatedAt: timestamp(at)})
}
func closeInstance(ctx context.Context, q *db.Queries, row db.AlertInstance, reason string, slot time.Time) error {
	var state InstanceState
	if json.Unmarshal(row.State, &state) != nil {
		return ErrInvalid
	}
	state.ClosureReason = reason
	state.PendingSince = nil
	state.RecoveringSince = nil
	raw, _ := json.Marshal(state)
	updated, err := q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.ID, State: raw, Lifecycle: "closed", Health: row.Health, Severity: row.Severity, StartedAt: row.StartedAt, ResolvedAt: timestamp(slot), LastEvaluatedAt: timestamp(slot), Version: row.Version})
	if err != nil {
		return err
	}
	_, err = appendLifecycleEvent(ctx, q, updated, reason, slot)
	return err
}
func (w Worker) healthEpisode(ctx context.Context, q *db.Queries, policy db.AlertConfiguration, p PolicySpec, result Evaluation) (db.AlertEvent, error) {
	fp := fingerprint(policy.ID, policy.Generation, "health:"+result.Health)
	row, err := q.GetActiveAlertFingerprint(ctx, db.GetActiveAlertFingerprintParams{EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID, Fingerprint: fp})
	if err == nil {
		return db.AlertEvent{}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.AlertEvent{}, err
	}
	sources, _ := json.Marshal(result.Sources)
	state := InstanceState{Name: "Query health: " + result.Health, PolicyName: policy.Name, Labels: map[string]string{"alert_health": result.Health, "policy_id": policy.ID.String()}, PolicySnapshot: policy.Spec, Kind: "health", NotificationState: "pending", LastSlot: result.Slot, NoiseReasons: []string{}, EvaluatorID: result.Evaluator.SubjectID, AuthorizationVersion: result.Evaluator.AuthorizationVersion, SourceSnapshot: sources}
	raw, _ := json.Marshal(state)
	row, err = q.CreateAlertInstance(ctx, db.CreateAlertInstanceParams{ID: uuid.New(), EnterpriseID: policy.EnterpriseID, PolicyID: policy.ID, PolicyVersion: policy.Version, Generation: policy.Generation, Fingerprint: fp, ResourceIds: result.ResourceIDs, SourceIds: []uuid.UUID{}, State: raw, Lifecycle: "firing", Health: result.Health, Severity: "warning", StartedAt: timestamp(result.Slot), LastEvaluatedAt: timestamp(result.Slot)})
	if err != nil {
		return db.AlertEvent{}, err
	}
	return appendLifecycleEvent(ctx, q, row, "first", result.Slot)
}
