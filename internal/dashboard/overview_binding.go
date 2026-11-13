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

type OverviewSelection struct {
	ClusterID        uuid.UUID     `json:"cluster_id"`
	ResourceVersion  int64         `json:"resource_version"`
	SelectionVersion int64         `json:"selection_version"`
	Mode             string        `json:"mode"`
	Dashboard        *db.Dashboard `json:"dashboard,omitempty"`
	DefaultDashboard db.Dashboard  `json:"default_dashboard"`
	UnavailableCode  string        `json:"unavailable_code,omitempty"`
}
type OverviewInput struct {
	DashboardID              uuid.UUID `json:"dashboard_id"`
	ExpectedDashboardVersion int64     `json:"expected_dashboard_version"`
	ExpectedSelectionVersion int64     `json:"expected_selection_version"`
	ExpectedResourceVersion  int64     `json:"expected_resource_version"`
	Reset                    bool      `json:"reset"`
}
type overviewPlan struct {
	OverviewInput
	ClusterID uuid.UUID `json:"cluster_id"`
	BindingID uuid.UUID `json:"binding_id"`
}

func overviewCompatible(spec Spec) bool {
	if len(spec.Panels) == 0 {
		return false
	}
	for _, panel := range spec.Panels {
		if !slices.Contains(panel.ApplicableResourceTypes, "kubernetes_cluster") && !slices.Contains(panel.ApplicableResourceTypes, "kubernetes_namespace") {
			return false
		}
	}
	return true
}
func (s Service) Overview(ctx context.Context, a Actor, cluster uuid.UUID, resourceVersion int64) (OverviewSelection, error) {
	out := OverviewSelection{ClusterID: cluster, ResourceVersion: resourceVersion, Mode: "default"}
	if err := authorize(ctx, s.Store.Queries, a, "telemetry.dashboard.read"); err != nil {
		return out, err
	}
	defaultBoard, err := s.ApplyManagedTemplate(ctx, a.EnterpriseID, OperationsOverviewTemplate)
	if err != nil {
		return out, err
	}
	out.DefaultDashboard = defaultBoard
	binding, err := s.Store.Queries.GetDashboardOverviewBinding(ctx, db.GetDashboardOverviewBindingParams{EnterpriseID: a.EnterpriseID, ResourceID: cluster})
	id := defaultBoard.ID
	if err == nil {
		out.SelectionVersion = binding.Version
		id = binding.DashboardID
		if id != defaultBoard.ID {
			out.Mode = "custom"
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	board, revision, err := s.Get(ctx, a, id)
	if err != nil {
		out.UnavailableCode = "DASHBOARD_UNAVAILABLE"
		return out, nil
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil || !overviewCompatible(spec) || board.Lifecycle != "active" {
		out.UnavailableCode = "DASHBOARD_UNAVAILABLE"
		return out, nil
	}
	out.Dashboard = &board
	return out, nil
}
func validateOverview(ctx context.Context, q *db.Queries, a Actor, plan overviewPlan, lock bool) (db.Dashboard, string, error) {
	var board db.Dashboard
	if plan.ClusterID == uuid.Nil || plan.DashboardID == uuid.Nil || plan.BindingID == uuid.Nil || plan.ExpectedDashboardVersion < 1 || plan.ExpectedSelectionVersion < 0 {
		return board, "", ErrInvalid
	}
	if plan.Reset && plan.DashboardID != templateDashboardID(a.EnterpriseID, OperationsOverviewTemplate) {
		return board, "", ErrInvalid
	}
	if err := authorize(ctx, q, a, "telemetry.dashboard.read"); err != nil {
		return board, "", err
	}
	if err := requireObject(ctx, q, a, "dashboard", plan.DashboardID); err != nil {
		return board, "", err
	}
	var err error
	if lock {
		board, err = q.LockDashboard(ctx, db.LockDashboardParams{EnterpriseID: a.EnterpriseID, ID: plan.DashboardID})
	} else {
		board, err = q.GetDashboard(ctx, db.GetDashboardParams{EnterpriseID: a.EnterpriseID, ID: plan.DashboardID})
	}
	if err != nil {
		return board, "", err
	}
	if board.Version != plan.ExpectedDashboardVersion {
		return board, "", ErrConflict
	}
	if board.Lifecycle != "active" || !board.ActiveRevisionID.Valid {
		return board, "", ErrArchived
	}
	name, version, err := bindingResource(ctx, q, a, "kubernetes_cluster", plan.ClusterID, true, lock)
	if err != nil {
		return board, "", err
	}
	if version != plan.ExpectedResourceVersion {
		return board, "", ErrConflict
	}
	binding, err := q.GetDashboardOverviewBinding(ctx, db.GetDashboardOverviewBindingParams{EnterpriseID: a.EnterpriseID, ResourceID: plan.ClusterID})
	current := int64(0)
	if err == nil {
		current = binding.Version
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return board, "", err
	}
	if current != plan.ExpectedSelectionVersion {
		return board, "", ErrConflict
	}
	revision, err := q.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{EnterpriseID: a.EnterpriseID, ID: board.ActiveRevisionID.UUID})
	if err != nil {
		return board, "", err
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil || !overviewCompatible(spec) {
		return board, "", ErrInvalid
	}
	return board, name, nil
}
func (s Service) PreviewOverview(ctx context.Context, a Actor, cluster uuid.UUID, in OverviewInput, key string) (db.PendingAction, error) {
	if key == "" {
		return db.PendingAction{}, ErrInvalid
	}
	plan := overviewPlan{OverviewInput: in, ClusterID: cluster, BindingID: uuid.NewSHA1(a.EnterpriseID, []byte("kubernetes-overview/"+cluster.String()))}
	board, name, err := validateOverview(ctx, s.Store.Queries, a, plan, false)
	if err != nil {
		return db.PendingAction{}, err
	}
	operation := "select"
	if in.Reset {
		operation = "reset"
	}
	return s.Actions.PrepareForSubject(ctx, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, resource.PrepareActionInput{
		ActionType: "telemetry.dashboard.overview." + operation, Title: board.Name, Summary: "Select the cluster overview dashboard", Risk: "write", ResourceType: "kubernetes_cluster", ResourceID: nullID(cluster), ExpectedResourceVersion: pgtype.Int8{Int64: in.ExpectedResourceVersion, Valid: true}, AuthorizationVersion: a.AuthorizationVersion,
		Preview: map[string]any{"name": board.Name, "resource_name": name, "operation": operation}, Diff: []Change{{Kind: "change", Text: name + " → " + board.Name}}, ImmutablePlan: plan, ResourceScopeSnapshot: plan, CommitHandler: "telemetry.dashboard.overview.commit",
	}, key)
}
func overviewState(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (overviewPlan, Actor, error) {
	actor := Actor{EnterpriseID: action.EnterpriseID, SubjectID: action.CreatorSubjectID, SubjectType: action.CreatorSubjectType, AuthorizationVersion: action.AuthorizationVersion}
	var plan overviewPlan
	if json.Unmarshal(raw, &plan) != nil || action.ResourceType != "kubernetes_cluster" || action.ResourceID != nullID(plan.ClusterID) {
		return plan, actor, ErrInvalid
	}
	expected := "telemetry.dashboard.overview.select"
	if plan.Reset {
		expected = "telemetry.dashboard.overview.reset"
	}
	if action.ActionType != expected {
		return plan, actor, ErrInvalid
	}
	_, _, err := validateOverview(ctx, q, actor, plan, true)
	return plan, actor, err
}
func revalidateOverview(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) ([]byte, error) {
	plan, _, err := overviewState(ctx, q, action, raw)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(plan)
	hash := sha256.Sum256(data)
	return hash[:], nil
}
func commitOverview(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	plan, actor, err := overviewState(ctx, q, action, raw)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	binding, err := q.PutDashboardOverviewBinding(ctx, db.PutDashboardOverviewBindingParams{ID: plan.BindingID, EnterpriseID: actor.EnterpriseID, DashboardID: plan.DashboardID, ResourceID: plan.ClusterID, CreatedBy: actor.SubjectID})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	board, err := q.GetDashboard(ctx, db.GetDashboardParams{ID: plan.DashboardID, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	err = recordBinding(ctx, q, actor, binding.ID, board.ID, "kubernetes_cluster", plan.ClusterID, board.Name, "overview", action.ActionRef)
	return resource.ActionCommitResult{ResourceType: "dashboard_binding", ResourceID: binding.ID, ResourceVersion: binding.Version, Summary: "Cluster overview dashboard selected"}, err
}
