package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type BindingInput struct {
	Operation                string    `json:"operation"`
	DashboardID              uuid.UUID `json:"dashboard_id"`
	ExpectedDashboardVersion int64     `json:"expected_dashboard_version"`
	ExpectedResourceVersion  int64     `json:"expected_resource_version"`
	BindingID                uuid.UUID `json:"binding_id,omitempty"`
	ExpectedBindingVersion   int64     `json:"expected_binding_version,omitempty"`
}
type BindingEntry struct {
	Binding         db.DashboardBinding
	Dashboard       db.Dashboard
	ResourceName    string
	ResourceVersion int64
}
type ResourceBindings struct {
	Name    string
	Version int64
	Items   []BindingEntry
}
type bindingPlan struct {
	BindingInput
	ResourceType string    `json:"resource_type"`
	ResourceID   uuid.UUID `json:"resource_id"`
	ID           uuid.UUID `json:"id"`
}

// Bindings are navigation metadata. Neither list nor attach grants access to either object.
func (service Service) ResourceBindings(ctx context.Context, actor Actor, kind string, id uuid.UUID) (ResourceBindings, error) {
	result := ResourceBindings{Items: []BindingEntry{}}
	q := service.Store.Queries
	if err := authorize(ctx, q, actor, "telemetry.dashboard.read"); err != nil {
		return result, err
	}
	name, version, err := bindingResource(ctx, q, actor, kind, id, false, false)
	if err != nil {
		return result, err
	}
	result.Name, result.Version = name, version
	rows, err := q.ListResourceDashboardBindings(ctx, db.ListResourceDashboardBindingsParams{EnterpriseID: actor.EnterpriseID, ResourceType: kind, ResourceID: id})
	if err != nil {
		return result, err
	}
	ids, err := authorizedIDs(ctx, q, actor, "dashboard")
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		if !slices.Contains(ids, row.DashboardID) {
			continue
		}
		item, e := q.GetDashboard(ctx, db.GetDashboardParams{ID: row.DashboardID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			return result, e
		}
		if item.Lifecycle != "active" {
			continue
		}
		result.Items = append(result.Items, BindingEntry{Binding: row, Dashboard: item, ResourceName: name, ResourceVersion: version})
	}
	return result, nil
}

func (service Service) DashboardBindings(ctx context.Context, actor Actor, id uuid.UUID) ([]BindingEntry, error) {
	item, _, err := service.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	result := []BindingEntry{}
	if item.Lifecycle != "active" {
		return result, nil
	}
	rows, err := service.Store.Queries.ListDashboardBindings(ctx, db.ListDashboardBindingsParams{EnterpriseID: actor.EnterpriseID, DashboardID: id})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		name, version, e := bindingResource(ctx, service.Store.Queries, actor, row.ResourceType, row.ResourceID, false, false)
		if errors.Is(e, ErrDenied) || errors.Is(e, ErrNotFound) {
			continue
		}
		if e != nil {
			return nil, e
		}
		result = append(result, BindingEntry{Binding: row, Dashboard: item, ResourceName: name, ResourceVersion: version})
	}
	return result, nil
}

func bindingResource(ctx context.Context, q *db.Queries, actor Actor, kind string, id uuid.UUID, manage, lock bool) (string, int64, error) {
	if id == uuid.Nil {
		return "", 0, ErrInvalid
	}
	permission := "host.read"
	switch kind {
	case "host":
		if manage {
			permission = "host.manage"
		}
	case "kubernetes_cluster":
		permission = "kubernetes.read"
		if manage {
			permission = "kubernetes.manage"
		}
	default:
		return "", 0, ErrInvalid
	}
	if err := authorize(ctx, q, actor, permission); err != nil {
		return "", 0, err
	}
	if err := requireObject(ctx, q, actor, kind, id); err != nil {
		return "", 0, err
	}
	var name, status string
	var version int64
	var err error
	if kind == "host" {
		if lock {
			var row db.LockDashboardBindingHostRow
			row, err = q.LockDashboardBindingHost(ctx, db.LockDashboardBindingHostParams{ID: id, EnterpriseID: actor.EnterpriseID})
			name, status, version = row.Name, row.Status, row.ResourceVersion
		} else {
			var row db.Host
			row, err = q.GetHost(ctx, db.GetHostParams{ID: id, EnterpriseID: actor.EnterpriseID})
			name, status, version = row.Name, row.Status, row.ResourceVersion
		}
	} else {
		if lock {
			var row db.LockDashboardBindingClusterRow
			row, err = q.LockDashboardBindingCluster(ctx, db.LockDashboardBindingClusterParams{ID: id, EnterpriseID: actor.EnterpriseID})
			name, status, version = row.Name, row.Status, row.ResourceVersion
		} else {
			var row db.KubernetesCluster
			row, err = q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: id, EnterpriseID: actor.EnterpriseID})
			name, status, version = row.Name, row.Status, row.ResourceVersion
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, err
	}
	if status != "active" {
		return "", 0, ErrDenied
	}
	return name, version, nil
}

func (service Service) PreviewBinding(ctx context.Context, actor Actor, kind string, id uuid.UUID, input BindingInput, key string) (db.PendingAction, error) {
	if key == "" {
		return db.PendingAction{}, ErrInvalid
	}
	plan := bindingPlan{BindingInput: input, ResourceType: kind, ResourceID: id, ID: input.BindingID}
	if input.Operation == "attach" {
		plan.ID = uuid.NewSHA1(actor.EnterpriseID, []byte(actor.SubjectType+":"+actor.SubjectID.String()+":"+key+":binding"))
	}
	name, item, err := validateBinding(ctx, service.Store.Queries, actor, plan, false)
	if err != nil {
		return db.PendingAction{}, err
	}
	return service.Actions.PrepareForSubject(ctx, resource.ActionSubject{ID: actor.SubjectID.String(), Type: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion, ConfirmationRequired: true}, actor.EnterpriseID, resource.PrepareActionInput{
		ActionType: "telemetry.dashboard.binding." + input.Operation, Title: item.Name, Summary: "Update resource dashboard shortcut", Risk: "write", ResourceType: kind, ResourceID: nullID(id), ExpectedResourceVersion: pgtype.Int8{Int64: input.ExpectedResourceVersion, Valid: true}, AuthorizationVersion: actor.AuthorizationVersion,
		Preview: map[string]any{"name": item.Name, "resource_name": name, "resource_type": kind, "operation": input.Operation}, Diff: []Change{{Kind: map[bool]string{true: "add", false: "remove"}[input.Operation == "attach"], Text: name + " → " + item.Name}}, ImmutablePlan: plan, ResourceScopeSnapshot: plan, CommitHandler: "telemetry.dashboard.binding.commit",
	}, key)
}

func validateBinding(ctx context.Context, q *db.Queries, actor Actor, plan bindingPlan, lock bool) (string, db.Dashboard, error) {
	var item db.Dashboard
	if plan.DashboardID == uuid.Nil || plan.ID == uuid.Nil || plan.ExpectedDashboardVersion < 1 || plan.ExpectedResourceVersion < 1 || plan.Operation != "attach" && plan.Operation != "detach" {
		return "", item, ErrInvalid
	}
	if plan.Operation == "attach" && (plan.BindingID != uuid.Nil || plan.ExpectedBindingVersion != 0) || plan.Operation == "detach" && (plan.BindingID != plan.ID || plan.ExpectedBindingVersion < 1) {
		return "", item, ErrInvalid
	}
	if err := authorize(ctx, q, actor, "telemetry.dashboard.read"); err != nil {
		return "", item, err
	}
	if err := requireObject(ctx, q, actor, "dashboard", plan.DashboardID); err != nil {
		return "", item, err
	}
	var err error
	if lock {
		item, err = q.LockDashboard(ctx, db.LockDashboardParams{ID: plan.DashboardID, EnterpriseID: actor.EnterpriseID})
	} else {
		item, err = q.GetDashboard(ctx, db.GetDashboardParams{ID: plan.DashboardID, EnterpriseID: actor.EnterpriseID})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", item, ErrNotFound
	}
	if err != nil {
		return "", item, err
	}
	if item.Version != plan.ExpectedDashboardVersion {
		return "", item, ErrConflict
	}
	if plan.Operation == "attach" && item.Lifecycle != "active" {
		return "", item, ErrArchived
	}
	name, version, err := bindingResource(ctx, q, actor, plan.ResourceType, plan.ResourceID, true, lock)
	if err != nil {
		return "", item, err
	}
	if version != plan.ExpectedResourceVersion {
		return "", item, ErrConflict
	}
	existing, err := q.FindDashboardBinding(ctx, db.FindDashboardBindingParams{EnterpriseID: actor.EnterpriseID, DashboardID: plan.DashboardID, ResourceType: plan.ResourceType, ResourceID: plan.ResourceID})
	if plan.Operation == "attach" {
		if err == nil {
			return "", item, ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", item, err
		}
	} else {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", item, ErrConflict
		}
		if err != nil {
			return "", item, err
		}
		if existing.ID != plan.BindingID || existing.Version != plan.ExpectedBindingVersion {
			return "", item, ErrConflict
		}
	}
	return name, item, nil
}

func bindingState(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (bindingPlan, Actor, error) {
	actor := Actor{EnterpriseID: action.EnterpriseID, SubjectID: action.CreatorSubjectID, SubjectType: action.CreatorSubjectType, AuthorizationVersion: action.AuthorizationVersion}
	var plan bindingPlan
	if json.Unmarshal(raw, &plan) != nil || action.ActionType != "telemetry.dashboard.binding."+plan.Operation || action.ResourceType != plan.ResourceType || action.ResourceID != nullID(plan.ResourceID) {
		return plan, actor, ErrInvalid
	}
	_, _, err := validateBinding(ctx, q, actor, plan, true)
	return plan, actor, err
}
func revalidateBinding(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) ([]byte, error) {
	plan, _, err := bindingState(ctx, q, action, raw)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(plan)
	hash := sha256.Sum256(encoded)
	return hash[:], nil
}
func commitBinding(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	plan, actor, err := bindingState(ctx, q, action, raw)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if plan.Operation == "attach" {
		_, err = q.CreateDashboardBinding(ctx, db.CreateDashboardBindingParams{ID: plan.ID, EnterpriseID: actor.EnterpriseID, DashboardID: plan.DashboardID, ResourceType: plan.ResourceType, ResourceID: plan.ResourceID, CreatedBy: actor.SubjectID})
	} else {
		var count int64
		count, err = q.DeleteDashboardBinding(ctx, db.DeleteDashboardBindingParams{ID: plan.BindingID, EnterpriseID: actor.EnterpriseID, Version: plan.ExpectedBindingVersion})
		if err == nil && count != 1 {
			err = ErrConflict
		}
	}
	if err == nil {
		item, e := q.GetDashboard(ctx, db.GetDashboardParams{ID: plan.DashboardID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			err = e
		} else {
			err = recordBinding(ctx, q, actor, plan.ID, plan.DashboardID, plan.ResourceType, plan.ResourceID, item.Name, plan.Operation, action.ActionRef)
		}
	}
	return resource.ActionCommitResult{ResourceType: "dashboard_binding", ResourceID: plan.ID, ResourceVersion: 1, Summary: plan.Operation}, err
}
