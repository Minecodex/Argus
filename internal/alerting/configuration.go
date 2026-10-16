package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}
func nullID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }
func timestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}
func configurationDTO(row db.AlertConfiguration) Configuration {
	return Configuration{ID: row.ID, Kind: row.Kind, Name: row.Name, Spec: row.Spec, Version: row.Version, Generation: row.Generation, Health: row.EvaluationHealth, ErrorCode: row.EvaluationError, LastEvaluatedAt: timePtr(row.LastEvaluatedAt), LastSuccessfulAt: timePtr(row.LastSuccessfulAt), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}
func (s Service) Configurations(ctx context.Context, a Actor, kind string) ([]Configuration, error) {
	if s.Store == nil {
		return nil, ErrUnavailable
	}
	permission := ConfigurationReadPermission(kind)
	if err := authorize(ctx, s.Store.Queries, a, permission); err != nil {
		return nil, err
	}
	rows, err := s.Store.Queries.ListAlertConfigurations(ctx, db.ListAlertConfigurationsParams{EnterpriseID: a.EnterpriseID, Kind: kind})
	if err != nil {
		return nil, err
	}
	allowed, err := actorResources(ctx, s.Store.Queries, a)
	if err != nil {
		return nil, err
	}
	items := []Configuration{}
	for _, row := range rows {
		ids, e := configurationResources(ctx, s.Store.Queries, a, row)
		if e != nil || !containsAll(allowed, ids) {
			continue
		}
		item := configurationDTO(row)
		if row.Kind == "silence" || row.Kind == "inhibition" {
			var enabled struct {
				Enabled bool `json:"enabled"`
			}
			if json.Unmarshal(row.Spec, &enabled) != nil {
				return nil, ErrInvalid
			}
			if enabled.Enabled {
				item.MatchedInstances, err = s.Store.Queries.CountAlertNoiseHits(ctx, db.CountAlertNoiseHitsParams{EnterpriseID: a.EnterpriseID, Column2: allowed, Column3: row.ID.String() + ":v%"})
				if err != nil {
					return nil, err
				}
			}
		}
		items = append(items, item)
	}
	return items, nil
}
func (s Service) Configuration(ctx context.Context, a Actor, id uuid.UUID) (Configuration, error) {
	row, err := s.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return Configuration{}, ErrNotFound
	}
	rows, err := s.Configurations(ctx, a, row.Kind)
	if err != nil {
		return Configuration{}, err
	}
	for _, value := range rows {
		if value.ID == id {
			return value, nil
		}
	}
	return Configuration{}, ErrDenied
}
func (s Service) Preview(ctx context.Context, a Actor, m Mutation, key string) (db.PendingAction, error) {
	if s.Store == nil || key == "" {
		return db.PendingAction{}, ErrUnavailable
	}
	if m.Operation == "create" {
		if m.ID != uuid.Nil || m.ExpectedVersion != 0 {
			return db.PendingAction{}, ErrInvalid
		}
		m.ID = uuid.NewSHA1(a.EnterpriseID, []byte("alert:"+a.SubjectID.String()+":"+m.Kind+":"+key))
		if slices.Contains([]string{"settings", "grouping", "template"}, m.Kind) {
			m.ID = uuid.NewSHA1(a.EnterpriseID, []byte("alert-singleton:"+m.Kind))
		}
	}
	if m.Operation == "delete" {
		row, err := s.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		if err != nil {
			return db.PendingAction{}, ErrNotFound
		}
		m.Name = row.Name
	}
	if m.Operation == "enable" || m.Operation == "disable" {
		row, err := s.Store.Queries.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		if err != nil || row.Kind != m.Kind {
			return db.PendingAction{}, ErrNotFound
		}
		body := map[string]any{}
		if json.Unmarshal(row.Spec, &body) != nil {
			return db.PendingAction{}, ErrInvalid
		}
		body["enabled"] = m.Operation == "enable"
		m.Spec, _ = json.Marshal(body)
		m.Name = row.Name
		m.Operation = "update"
	}
	var err error
	m, err = normalizeMutation(m)
	if err != nil {
		return db.PendingAction{}, err
	}
	// Deterministic rule identities make a recovered preview idempotent.
	if m.Kind == "policy" && m.Operation == "create" {
		p, _ := decodeSpec[PolicySpec](m.Spec)
		for i := range p.Rules {
			p.Rules[i].ID = uuid.NewSHA1(m.ID, []byte("rule:"+string(rune(i+65))))
		}
		m.Spec, _ = json.Marshal(p)
	}
	plan, err := preparePlan(ctx, s.Store.Queries, a, m, false)
	if err != nil {
		return db.PendingAction{}, err
	}
	return retryAlertWrite(ctx, func() (db.PendingAction, error) {
		return s.Actions.PrepareForSubject(ctx, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, resource.PrepareActionInput{
			ActionType: "alert." + m.Kind + "." + m.Operation, Title: m.Name, Summary: "Apply alert configuration", Risk: "write", ResourceType: "alert_" + m.Kind, ResourceID: nullID(m.ID), ExpectedResourceVersion: pgtype.Int8{Int64: m.ExpectedVersion, Valid: m.ExpectedVersion > 0}, AuthorizationVersion: a.AuthorizationVersion, RunID: nullID(a.RunID),
			Preview: previewInvocation(a, key, map[string]any{"kind": m.Kind, "operation": m.Operation, "id": m.ID, "name": m.Name, "spec": previewSpec(m.Spec), "resource_ids": plan.ResourceIDs, "scope_mode": "explicit authorization with dynamic matching"}), Diff: []map[string]string{{"kind": "change", "text": m.Operation + " " + m.Kind + ": " + m.Name}}, ImmutablePlan: plan, ResourceScopeSnapshot: planScope(plan), CommitHandler: "alert." + m.Kind + "." + m.Operation + ".commit",
		}, key)
	})
}
func preparePlan(ctx context.Context, q *db.Queries, a Actor, m Mutation, lock bool) (ActionPlan, error) {
	plan := ActionPlan{Mutation: m, Actor: a, ResourceIDs: []uuid.UUID{}, References: map[string]int64{}}
	if err := authorize(ctx, q, a, requiredPermission(m.Kind)); err != nil {
		return plan, err
	}
	if m.Operation != "create" {
		var row db.AlertConfiguration
		var err error
		if lock {
			row, err = q.LockAlertConfiguration(ctx, db.LockAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		} else {
			row, err = q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		}
		if err != nil || row.Kind != m.Kind {
			return plan, ErrNotFound
		}
		if row.Version != m.ExpectedVersion {
			return plan, ErrConflict
		}
		prior, err := configurationResources(ctx, q, a, row)
		allowed, e := actorResources(ctx, q, a)
		if err != nil {
			return plan, err
		}
		if e != nil || !containsAll(allowed, prior) {
			return plan, ErrDenied
		}
		if m.Operation == "delete" {
			plan.ResourceIDs = prior
		}
	}
	if m.Operation == "delete" {
		return plan, nil
	}
	switch m.Kind {
	case "policy":
		p, err := decodeSpec[PolicySpec](m.Spec)
		if err != nil {
			return plan, err
		}
		eval, err := evaluator(ctx, q, a.EnterpriseID)
		if err != nil && p.Enabled {
			return plan, err
		}
		if err != nil {
			eval, err = configuredEvaluator(ctx, q, a.EnterpriseID)
			if err != nil {
				return plan, err
			}
		}
		ids, err := selectorResources(ctx, q, eval, p.Resources, p.Enabled)
		if err != nil {
			return plan, err
		}
		allowed, err := actorResources(ctx, q, a)
		if err != nil || !containsAll(allowed, ids) {
			return plan, ErrDenied
		}
		plan.ResourceIDs = ids
		plan.EvaluatorID = eval.SubjectID
		plan.EvaluatorVersion = eval.AuthorizationVersion
		for _, id := range p.ChannelIDs {
			row, e := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: id})
			if e != nil || row.Kind != "channel" {
				return plan, ErrInvalid
			}
			plan.References["channel:"+id.String()] = row.Version
			if err := checkReference(ctx, q, a.EnterpriseID, id, "channel"); err != nil {
				return plan, err
			}
		}
		for _, id := range p.RouteIDs {
			row, e := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: id})
			if e != nil || row.Kind != "route" {
				return plan, ErrInvalid
			}
			plan.References["route:"+id.String()] = row.Version
			if err := checkReference(ctx, q, a.EnterpriseID, id, "route"); err != nil {
				return plan, err
			}
		}
	case "silence", "inhibition":
		p, err := decodeSpec[SilenceSpec](m.Spec)
		if m.Kind == "inhibition" {
			i, e := decodeSpec[InhibitionSpec](m.Spec)
			err = e
			p = SilenceSpec{ResourceIDs: i.ResourceIDs, EndsAt: time.Now().Add(time.Hour)}
		}
		if err != nil {
			return plan, err
		}
		if !p.EndsAt.After(time.Now().UTC()) {
			return plan, ErrInvalid
		}
		ids, err := actorResources(ctx, q, a)
		if err != nil {
			return plan, err
		}
		if len(p.ResourceIDs) == 0 {
			eval, e := evaluator(ctx, q, a.EnterpriseID)
			if e != nil {
				return plan, e
			}
			plan.ResourceIDs, e = actorResources(ctx, q, eval)
			if e != nil {
				return plan, e
			}
		} else {
			plan.ResourceIDs = p.ResourceIDs
		}
		if !containsAll(ids, plan.ResourceIDs) {
			return plan, ErrDenied
		}
		for _, id := range p.PolicyIDs {
			if err := checkReference(ctx, q, a.EnterpriseID, id, "policy"); err != nil {
				return plan, err
			}
		}
	case "grouping", "template":
		ids, e := globalConfigurationScope(ctx, q, a)
		if e != nil {
			return plan, e
		}
		plan.ResourceIDs = ids
	case "settings":
		p, _ := decodeSpec[SettingsSpec](m.Spec)
		account, err := q.GetServiceAccount(ctx, db.GetServiceAccountParams{EnterpriseID: a.EnterpriseID, ID: p.EvaluatorID})
		if err != nil || account.Status != "active" {
			return plan, ErrEvaluator
		}
		plan.EvaluatorID = account.ID
		plan.EvaluatorVersion = account.AuthorizationVersion
	case "channel":
		p, _ := decodeSpec[ChannelSpec](m.Spec)
		if err := authorize(ctx, q, a, "credential.use"); err != nil {
			return plan, err
		}
		credential, err := q.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: a.EnterpriseID, ID: p.CredentialID})
		if err != nil || credential.Status != "active" || credential.Protocol != "notification" {
			return plan, ErrInvalid
		}
		plan.References["credential:"+credential.ID.String()] = credential.Version
		v, e := q.GetCurrentSecretVersion(ctx, db.GetCurrentSecretVersionParams{EnterpriseID: a.EnterpriseID, SecretID: credential.SecretID})
		if e != nil {
			return plan, ErrInvalid
		}
		plan.References["secret:"+credential.SecretID.String()] = int64(v.Version)
	case "route":
		ids, e := globalConfigurationScope(ctx, q, a)
		if e != nil {
			return plan, e
		}
		plan.ResourceIDs = ids
		p, _ := decodeSpec[RouteSpec](m.Spec)
		for _, id := range p.ChannelIDs {
			row, e := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: a.EnterpriseID, ID: id})
			if e != nil || row.Kind != "channel" {
				return plan, ErrInvalid
			}
			plan.References["channel:"+id.String()] = row.Version
			if err := checkReference(ctx, q, a.EnterpriseID, id, "channel"); err != nil {
				return plan, err
			}
		}
	}
	return plan, nil
}
func checkReference(ctx context.Context, q *db.Queries, enterprise, id uuid.UUID, kind string) error {
	row, err := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: enterprise, ID: id})
	if err != nil || row.Kind != kind {
		return ErrInvalid
	}
	return nil
}
func planScope(plan ActionPlan) any {
	raw := plan.Mutation.Spec
	// A nil RawMessage becomes JSON null in the immutable plan. Both forms
	// represent absent deletion input and must produce the same impact hash.
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	canonical, _ := resource.CanonicalJSON(raw)
	specHash := sha256.Sum256(canonical)
	mutation := plan.Mutation
	mutation.Spec = nil
	return struct {
		References       map[string]int64
		SpecHash         [32]byte
		Mutation         Mutation
		Resources        []uuid.UUID
		Evaluator        uuid.UUID
		EvaluatorVersion int64
	}{plan.References, specHash, mutation, plan.ResourceIDs, plan.EvaluatorID, plan.EvaluatorVersion}
}

type ActionExtension struct{ Next resource.ActionExtension }

func (x ActionExtension) RevalidateAction(ctx context.Context, q *db.Queries, pending db.PendingAction, raw json.RawMessage) ([]byte, error) {
	if strings.HasPrefix(pending.ActionType, "alert.command.") {
		plan, err := revalidateCommand(ctx, q, pending, raw)
		if err != nil {
			return nil, err
		}
		value, _ := json.Marshal(plan)
		digest := sha256.Sum256(value)
		return digest[:], nil
	}

	if !strings.HasPrefix(pending.ActionType, "alert.") {
		if x.Next == nil {
			return nil, resource.ErrActionInvalidated
		}
		return x.Next.RevalidateAction(ctx, q, pending, raw)
	}
	var plan ActionPlan
	if json.Unmarshal(raw, &plan) != nil || plan.Actor.EnterpriseID != pending.EnterpriseID || plan.Actor.SubjectID != pending.CreatorSubjectID || plan.Actor.AuthorizationVersion != pending.AuthorizationVersion {
		return nil, resource.ErrActionInvalidated
	}
	current, err := preparePlan(ctx, q, plan.Actor, plan.Mutation, true)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(planScope(current))
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(value)
	return hash[:], nil
}
func (x ActionExtension) CommitAction(ctx context.Context, q *db.Queries, pending db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	if strings.HasPrefix(pending.ActionType, "alert.command.") {
		return commitCommand(ctx, q, pending, raw)
	}

	if !strings.HasPrefix(pending.ActionType, "alert.") {
		if x.Next == nil {
			return resource.ActionCommitResult{}, resource.ErrActionInvalidated
		}
		return x.Next.CommitAction(ctx, q, pending, raw)
	}
	var plan ActionPlan
	if json.Unmarshal(raw, &plan) != nil {
		return resource.ActionCommitResult{}, ErrInvalid
	}
	if _, err := preparePlan(ctx, q, plan.Actor, plan.Mutation, true); err != nil {
		return resource.ActionCommitResult{}, err
	}
	m := plan.Mutation
	var row db.AlertConfiguration
	var err error
	switch m.Operation {
	case "create":
		row, err = q.CreateAlertConfiguration(ctx, db.CreateAlertConfigurationParams{ID: m.ID, EnterpriseID: pending.EnterpriseID, Kind: m.Kind, Name: m.Name, Spec: m.Spec, CreatedBy: plan.Actor.SubjectID})
	case "update":
		old, e := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: pending.EnterpriseID, ID: m.ID})
		if e != nil {
			return resource.ActionCommitResult{}, e
		}
		generation := old.Generation
		if m.Kind == "policy" && evaluationIdentity(old.Spec) != evaluationIdentity(m.Spec) {
			generation++
		}
		row, err = q.UpdateAlertConfiguration(ctx, db.UpdateAlertConfigurationParams{EnterpriseID: pending.EnterpriseID, ID: m.ID, Name: m.Name, Spec: m.Spec, Generation: generation, Version: m.ExpectedVersion})
	case "delete":
		row, err = q.DeleteAlertConfiguration(ctx, db.DeleteAlertConfigurationParams{EnterpriseID: pending.EnterpriseID, ID: m.ID, Version: m.ExpectedVersion})
	default:
		err = ErrInvalid
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrConflict
	}
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if row.Kind == "policy" {
		p, _ := decodeSpec[PolicySpec](row.Spec)
		episodes, e := q.ListPolicyAlertInstances(ctx, db.ListPolicyAlertInstancesParams{EnterpriseID: row.EnterpriseID, PolicyID: row.ID})
		if e != nil {
			return resource.ActionCommitResult{}, e
		}
		for _, episode := range episodes {
			if row.Status == "deleted" || !p.Enabled || episode.Generation != row.Generation {
				reason := "configuration_changed"
				if row.Status == "deleted" {
					reason = "policy_deleted"
				} else if !p.Enabled {
					reason = "policy_disabled"
				}
				if e = closeInstance(ctx, q, episode, reason, time.Now().UTC()); e != nil {
					return resource.ActionCommitResult{}, e
				}
			}
		}
	}
	if err = q.AddAlertConfigurationRevision(ctx, db.AddAlertConfigurationRevisionParams{ConfigurationID: row.ID, EnterpriseID: row.EnterpriseID, Version: row.Version, Name: row.Name, Spec: row.Spec, ActorID: plan.Actor.SubjectID}); err != nil {
		return resource.ActionCommitResult{}, err
	}
	details, err := resource.PendingExecutionAuditDetails(ctx, q, pending, map[string]any{"version": row.Version, "kind": row.Kind, "resource_ids": plan.ResourceIDs})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if err = appendAudit(ctx, q, plan.Actor, pending.ActionType, row.ID, details); err != nil {
		return resource.ActionCommitResult{}, err
	}
	return resource.ActionCommitResult{ResourceType: "alert_" + row.Kind, ResourceID: row.ID, ResourceVersion: row.Version, Summary: "Alert configuration committed"}, nil
}
func evaluationIdentity(raw []byte) [32]byte {
	p, _ := decodeSpec[PolicySpec](raw)
	for i := range p.Rules {
		p.Rules[i].Severity = ""
		p.Rules[i].Name = ""
		p.Rules[i].Unit = ""
	}
	value, _ := json.Marshal([]any{p.Resources, p.Rules, p.Mode, p.MatchBy, p.IntervalSeconds, p.ForSeconds, p.RecoverySeconds, p.Enabled})
	return sha256.Sum256(value)
}
func appendAudit(ctx context.Context, q *db.Queries, a Actor, operation string, id uuid.UUID, details map[string]any) error {
	if _, ok := details["resource_ids"]; !ok {
		details["resource_ids"] = []uuid.UUID{}
	}
	actorType := a.SubjectType
	if actorType == "user" {
		actorType = "enterprise_user"
	}
	if a.RunID != uuid.Nil {
		details["origin"] = "model_agent"
		details["run_id"] = a.RunID.String()
	}
	_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: nullID(a.EnterpriseID), ActorType: actorType, ActorID: a.SubjectID.String(), Action: operation, ResourceType: "alert", ResourceID: id.String(), Result: "success", Details: details})
	return err
}

func previewInvocation(a Actor, invocation string, preview map[string]any) map[string]any {
	if a.RunID != uuid.Nil && uuid.Validate(invocation) == nil {
		preview["tool_call_id"] = invocation
	}
	return preview
}
