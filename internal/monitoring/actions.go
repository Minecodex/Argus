package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var environmentPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func lookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) {
		switch pgerr.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22001":
			return ErrInvalid
		}
	}
	return err
}

func validName(value string, max int) bool {
	return strings.TrimSpace(value) == value && value != "" && utf8.RuneCountInString(value) <= max && !strings.ContainsAny(value, "\x00\r\n")
}
func nullID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }

func (s Service) Preview(ctx context.Context, a Actor, m Mutation, key string) (db.PendingAction, error) {
	return s.PreviewWithCredentials(ctx, a, m, nil, key)
}

func (s Service) PreviewWithCredentials(ctx context.Context, a Actor, m Mutation, credentials json.RawMessage, key string) (db.PendingAction, error) {
	if s.Store == nil || key == "" || len(key) > 200 {
		return db.PendingAction{}, ErrInvalid
	}
	if m.Collection != nil && m.Collection.CredentialDraftID != uuid.Nil {
		return db.PendingAction{}, ErrInvalid
	}
	if m.Operation == "create" {
		if m.ID != uuid.Nil || m.ExpectedVersion != 0 {
			return db.PendingAction{}, ErrInvalid
		}
		m.ID = uuid.NewSHA1(a.EnterpriseID, []byte("monitoring:"+a.SubjectType+":"+a.SubjectID.String()+":"+m.Kind+":"+key))
		if m.Kind == "collection" && m.Collection != nil {
			if _, ok := builtincollection.Lookup(m.Collection.Integration); ok {
				m.ID = builtincollection.StableID(m.Collection.Target.Kind, m.Collection.Target.ID, m.Collection.Integration)
			}
		}
	}
	return s.previewTransaction(ctx, a, m, credentials, key)
}

func pendingInput(plan actionPlan) resource.PrepareActionInput {
	m, a := plan.Mutation, plan.Actor
	name := plan.Mutation.Kind
	if plan.Mutation.Application != nil {
		name = plan.Mutation.Application.Name
	}
	if m.Collection != nil {
		name = m.Collection.Name
	}
	if m.Service != nil {
		name = m.Service.Name
	}
	actionType := "monitoring." + m.Kind + "." + m.Operation
	if m.Kind == "application" {
		actionType = "application." + m.Operation
	}
	return resource.PrepareActionInput{
		ActionType: actionType, Title: name, Summary: "Apply monitoring workspace change", Risk: "write", ResourceType: m.Kind, ResourceID: nullID(m.ID),
		ExpectedResourceVersion: pgtype.Int8{Int64: m.ExpectedVersion, Valid: m.ExpectedVersion > 0}, AuthorizationVersion: a.AuthorizationVersion, RunID: nullID(a.RunID),
		Preview: map[string]any{"kind": m.Kind, "operation": m.Operation, "id": m.ID, "application": plan.Mutation.Application, "service": m.Service, "binding": plan.Mutation.Binding, "collection": publicCollectionInput(plan.Mutation.Collection), "endpoint": m.Endpoint, "ingest_key": m.IngestKey},
		Diff:    []map[string]string{{"kind": "change", "text": m.Operation + " " + name}}, ImmutablePlan: plan, ResourceScopeSnapshot: plan.References, CommitHandler: actionType + ".commit",
	}
}

func (s Service) prepare(ctx context.Context, q *db.Queries, a Actor, m Mutation, lock bool) (actionPlan, error) {
	p := actionPlan{SchemaVersion: "argus.monitoring_action/v1", Mutation: m, Actor: a, References: map[string]int64{}}
	if m.ID == uuid.Nil {
		return p, ErrInvalid
	}
	if m.Service != nil && m.Kind != "service" {
		return p, ErrInvalid
	}
	if (m.Endpoint != nil || m.IngestKey != nil) && m.Kind != "endpoint" && m.Kind != "ingest_key" {
		return p, ErrInvalid
	}
	switch m.Kind {
	case "service":
		return s.prepareServiceDeclaration(ctx, q, a, m, lock)
	case "endpoint", "ingest_key":
		return s.prepareIngest(ctx, q, a, m, lock)
	case "collection":
		return s.prepareCollection(ctx, q, a, m, lock)
	case "application":
		if err := authorization.Authorize(ctx, q, a, "application.manage"); err != nil {
			return p, err
		}
		if m.Binding != nil || m.Collection != nil {
			return p, ErrInvalid
		}
		if m.Operation != "create" && m.Operation != "update" && m.Operation != "archive" {
			return p, ErrInvalid
		}
		input := m.Application
		if m.Operation != "create" {
			if err := authorization.RequireResource(ctx, q, a, "application", m.ID); err != nil {
				return p, err
			}
			var row db.MonitoringApplication
			var err error
			if lock {
				row, err = q.LockMonitoringApplication(ctx, db.LockMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
			} else {
				row, err = q.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
			}
			if err != nil {
				return p, lookupError(err)
			}
			if row.Version != m.ExpectedVersion || row.Status != "active" {
				return p, ErrConflict
			}
			p.References["application:"+m.ID.String()] = row.Version
			if m.Operation == "archive" {
				count, err := q.CountApplicationCollectionDependencies(ctx, db.CountApplicationCollectionDependenciesParams{EnterpriseID: a.EnterpriseID, ApplicationID: nullID(m.ID)})
				if err != nil {
					return p, err
				}
				if count > 0 {
					return p, ErrDependency
				}
				input = &ApplicationInput{Key: row.ApplicationKey, Name: row.Name, Environment: row.Environment, Description: row.Description, OwnerName: row.OwnerName}
			} else {
				if input == nil {
					return p, ErrInvalid
				}
				copy := *input
				input = &copy
				if input.Environment == "" {
					input.Environment = row.Environment
				}
				if input.Key == "" {
					input.Key = row.ApplicationKey
				}
				if input.Environment != row.Environment || input.Key != row.ApplicationKey {
					return p, ErrInvalid
				}
			}
		}
		if input == nil {
			return p, ErrInvalid
		}
		copy := *input
		input = &copy
		if input.Key == "" {
			input.Key = m.ID.String()
		}
		if !validName(input.Key, 100) || !validName(input.Name, 200) || !environmentPattern.MatchString(input.Environment) || utf8.RuneCountInString(input.Description) > 4096 || utf8.RuneCountInString(input.OwnerName) > 200 {
			return p, ErrInvalid
		}
		p.Mutation.Application = input
	case "binding":
		if err := authorization.Authorize(ctx, q, a, "application.manage"); err != nil {
			return p, err
		}
		if m.Application != nil || m.Collection != nil {
			return p, ErrInvalid
		}
		input := m.Binding
		if m.Operation == "delete" {
			row, err := q.GetMonitoringBinding(ctx, db.GetMonitoringBindingParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
			if err != nil {
				return p, lookupError(err)
			}
			if row.Version != m.ExpectedVersion || row.ValidTo.Valid {
				return p, ErrConflict
			}
			input = &BindingInput{ApplicationID: row.ApplicationID, ServiceID: row.ServiceID.UUID, Target: ResourceRef{Kind: row.TargetType, ID: row.TargetID}}
			p.References["binding:"+row.ID.String()] = row.Version
		} else if m.Operation != "create" {
			return p, ErrInvalid
		}
		if input == nil {
			return p, ErrInvalid
		}
		if err := authorization.RequireResource(ctx, q, a, "application", input.ApplicationID); err != nil {
			return p, err
		}
		app, err := q.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: input.ApplicationID})
		if err != nil {
			return p, lookupError(err)
		}
		if app.Status != "active" {
			return p, ErrInvalid
		}
		p.References["application:"+app.ID.String()] = app.Version
		if input.ServiceID != uuid.Nil {
			service, err := q.GetMonitoringService(ctx, db.GetMonitoringServiceParams{EnterpriseID: a.EnterpriseID, ID: input.ServiceID})
			if err != nil || service.ApplicationID != app.ID || service.Status != "active" {
				return p, ErrInvalid
			}
			p.References["service:"+service.ID.String()] = service.Version
		}
		version, err := requireTarget(ctx, q, a, input.Target)
		if err != nil {
			return p, err
		}
		p.References[input.Target.Kind+":"+input.Target.ID.String()] = version
		p.Mutation.Binding = input
	default:
		return p, ErrUnsupported
	}
	return p, nil
}

func requireTarget(ctx context.Context, q *db.Queries, a Actor, ref ResourceRef) (int64, error) {
	permission := "monitoring.read"
	switch ref.Kind {
	case "host":
		permission = "host.read"
	case "kubernetes_cluster":
		permission = "kubernetes.read"
	case "monitoring_target":
	default:
		return 0, ErrInvalid
	}
	if err := authorization.Authorize(ctx, q, a, permission); err != nil {
		return 0, err
	}
	if err := authorization.RequireResource(ctx, q, a, ref.Kind, ref.ID); err != nil {
		return 0, err
	}
	switch ref.Kind {
	case "host":
		row, err := q.GetHost(ctx, db.GetHostParams{EnterpriseID: a.EnterpriseID, ID: ref.ID})
		if err != nil {
			return 0, lookupError(err)
		}
		if row.Status != "active" {
			return 0, ErrInvalid
		}
		return row.ResourceVersion, nil
	case "kubernetes_cluster":
		row, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: a.EnterpriseID, ID: ref.ID})
		if err != nil {
			return 0, lookupError(err)
		}
		if row.Status != "active" {
			return 0, ErrInvalid
		}
		return row.ResourceVersion, nil
	default:
		row, err := q.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: a.EnterpriseID, ID: ref.ID})
		if err != nil {
			return 0, lookupError(err)
		}
		if row.Status != "active" {
			return 0, ErrInvalid
		}
		return row.Version, nil
	}
}

type ActionExtension struct {
	Next    resource.ActionExtension
	Service Service
}

func (e ActionExtension) RevalidateAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) ([]byte, error) {
	if !monitoringAction(action) {
		if e.Next != nil {
			return e.Next.RevalidateAction(ctx, q, action, raw)
		}
		return nil, resource.ErrActionInvalidated
	}
	var prior actionPlan
	if json.Unmarshal(raw, &prior) != nil || prior.SchemaVersion != "argus.monitoring_action/v1" {
		return nil, ErrInvalid
	}
	a := actionActor(action)
	if prior.Actor.EnterpriseID != a.EnterpriseID || prior.Actor.SubjectID != a.SubjectID || prior.Actor.SubjectType != a.SubjectType {
		return nil, ErrDenied
	}
	if snapshot, ok := ctx.Value(workloadSnapshotKey{}).(workloadSnapshot); ok {
		canonical, err := resource.CanonicalJSON(raw)
		if err != nil || snapshot.actionID != action.ID || snapshot.planHash != sha256.Sum256(canonical) {
			return nil, resource.ErrActionInvalidated
		}
	}
	current, err := e.Service.prepare(ctx, q, a, prior.Mutation, true)
	if err != nil {
		return nil, err
	}
	// Report only a bounded reference category, never a plan, endpoint,
	// credential reference or hash. Public error contracts remain unchanged.
	for key, version := range prior.References {
		if current.References[key] != version {
			kind, _, _ := strings.Cut(key, ":")
			switch kind {
			case "collector", "runtime", "collection", "application", "service", "distribution", "kubernetes_namespace", "monitoring_target":
			default:
				kind = "scope"
			}
			return nil, fmt.Errorf("monitoring action reference changed (%s): %w", kind, resource.ErrActionInvalidated)
		}
	}
	encoded, err := json.Marshal(current.References)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	return digest[:], nil
}

func (e ActionExtension) CommitAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	if !monitoringAction(action) {
		if e.Next != nil {
			return e.Next.CommitAction(ctx, q, action, raw)
		}
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	var plan actionPlan
	if json.Unmarshal(raw, &plan) != nil || plan.SchemaVersion != "argus.monitoring_action/v1" {
		return resource.ActionCommitResult{}, ErrInvalid
	}
	result, err := e.Service.commitWorkspace(ctx, q, plan, action.ID)
	if err != nil {
		return result, lookupError(err)
	}
	for _, runtimePlan := range plan.RuntimePlans {
		if e.Service.Runtime == nil {
			return result, ErrUnavailable
		}
		if _, err = e.Service.Runtime.Commit(ctx, q, action, runtimePlan); err != nil {
			return result, err
		}
	}
	actorType := action.CreatorSubjectType
	if actorType == "user" {
		actorType = "enterprise_user"
	}
	_, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: nullID(action.EnterpriseID), ActorType: actorType, ActorID: action.CreatorSubjectID.String(), Action: action.ActionType, ResourceType: result.ResourceType, ResourceID: result.ResourceID.String(), Result: "success", Details: map[string]any{"version": result.ResourceVersion, "summary": result.Summary}})
	return result, err
}

func monitoringAction(a db.PendingAction) bool {
	return strings.HasPrefix(a.ActionType, "application.") || strings.HasPrefix(a.ActionType, "monitoring.")
}

func (s Service) commitWorkspace(ctx context.Context, q *db.Queries, p actionPlan, approvedAction uuid.UUID) (resource.ActionCommitResult, error) {
	m, a := p.Mutation, p.Actor
	result := resource.ActionCommitResult{ResourceType: m.Kind, ResourceID: m.ID, Summary: "Monitoring workspace change applied"}
	switch m.Kind {
	case "service":
		if m.Service == nil || m.Operation != "create" {
			return result, ErrInvalid
		}
		row, err := q.DeclareMonitoringService(ctx, db.DeclareMonitoringServiceParams{ID: m.ID, EnterpriseID: a.EnterpriseID, ApplicationID: m.Service.ApplicationID, ServiceNamespace: m.Service.Namespace, ServiceName: m.Service.Name})
		if err != nil {
			return result, lookupError(err)
		}
		result.ResourceVersion = row.Version
	case "endpoint", "ingest_key":
		return s.commitIngest(ctx, q, p)
	case "collection":
		return s.commitCollection(ctx, q, p, approvedAction)
	case "application":
		in := m.Application
		if in == nil {
			return result, ErrInvalid
		}
		if m.Operation == "create" {
			row, err := q.CreateMonitoringApplication(ctx, db.CreateMonitoringApplicationParams{ID: m.ID, EnterpriseID: a.EnterpriseID, ApplicationKey: in.Key, Name: in.Name, Environment: in.Environment, Description: in.Description, OwnerName: in.OwnerName, CreatedBy: a.SubjectID})
			if err != nil {
				return result, err
			}
			result.ResourceVersion = row.Version
			if err = authorization.GrantCreatedResource(ctx, q, a, "application", row.ID); err != nil {
				return result, err
			}
		} else {
			status := "active"
			if m.Operation == "archive" {
				status = "archived"
			}
			row, err := q.UpdateMonitoringApplication(ctx, db.UpdateMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: m.ID, Version: m.ExpectedVersion, Name: in.Name, Description: in.Description, OwnerName: in.OwnerName, Status: status})
			if err != nil {
				return result, err
			}
			result.ResourceVersion = row.Version
			if status == "archived" {
				if err = q.DisableApplicationIngest(ctx, db.DisableApplicationIngestParams{EnterpriseID: a.EnterpriseID, ApplicationID: m.ID}); err != nil {
					return result, err
				}
			}
		}
	case "binding":
		if m.Binding == nil {
			return result, ErrInvalid
		}
		if m.Operation == "create" {
			row, err := q.CreateMonitoringBinding(ctx, db.CreateMonitoringBindingParams{ID: m.ID, EnterpriseID: a.EnterpriseID, ApplicationID: m.Binding.ApplicationID, ServiceID: nullID(m.Binding.ServiceID), TargetType: m.Binding.Target.Kind, TargetID: m.Binding.Target.ID})
			if err != nil {
				return result, err
			}
			result.ResourceVersion = row.Version
		} else {
			rows, err := q.EndMonitoringBinding(ctx, db.EndMonitoringBindingParams{EnterpriseID: a.EnterpriseID, ID: m.ID, Version: m.ExpectedVersion})
			if err != nil {
				return result, err
			}
			if rows != 1 {
				return result, ErrConflict
			}
			result.ResourceVersion = m.ExpectedVersion + 1
		}
	default:
		return result, ErrUnsupported
	}
	return result, nil
}
