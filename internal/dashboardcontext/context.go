// Package dashboardcontext owns user-selected Dashboard references and their
// immutable Run scope. It depends on object authorization, not query engines.
package dashboardcontext

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type Selection struct {
	Mode            string      `json:"mode"`
	DashboardIDs    []uuid.UUID `json:"dashboard_ids"`
	ExpectedVersion *int64      `json:"expected_version,omitempty"`
}
type Snapshot struct {
	Schema       string      `json:"schema_version"`
	Mode         string      `json:"mode"`
	DashboardIDs []uuid.UUID `json:"dashboard_ids"`
	Version      int64       `json:"version"`
}
type Prepared struct {
	Snapshot    Snapshot
	BaseVersion int64
	Changed     bool
}

func None() Snapshot {
	return Snapshot{Schema: "argus.dashboard_context/v1", Mode: "none", DashboardIDs: []uuid.UUID{}}
}
func invalid() error  { return toolruntime.Error{Kind: "DASHBOARD_INVALID"} }
func conflict() error { return toolruntime.Error{Kind: "TOOL_CONFIGURATION_CHANGED"} }
func Current(ctx context.Context, q *db.Queries, p toolruntime.Principal) (Snapshot, error) {
	row, err := q.GetConversationDashboardContext(ctx, db.GetConversationDashboardContextParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return None(), nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	var result Snapshot
	if json.Unmarshal(row.Selection, &result) != nil || result.Version != row.Version || result.Schema != "argus.dashboard_context/v1" {
		return result, invalid()
	}
	return result, nil
}
func Validate(ctx context.Context, q *db.Queries, p toolruntime.Principal, s Snapshot) error {
	if s.Schema != "argus.dashboard_context/v1" || len(s.DashboardIDs) > 20 || s.Version < 0 {
		return invalid()
	}
	if !slices.Contains([]string{"none", "analyze", "create"}, s.Mode) || s.Mode == "none" && len(s.DashboardIDs) > 0 {
		return invalid()
	}
	if s.Mode == "none" {
		return nil
	}
	actor := dashboardaccess.Actor{EnterpriseID: p.EnterpriseID, SubjectID: p.UserID, SubjectType: "user", AuthorizationVersion: p.AuthorizationVersion}
	if err := dashboardaccess.Authorize(ctx, q, actor, "telemetry.dashboard.read"); err != nil {
		return err
	}
	if s.Mode == "create" {
		if err := dashboardaccess.Authorize(ctx, q, actor, "telemetry.dashboard.manage"); err != nil {
			return err
		}
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range s.DashboardIDs {
		if id == uuid.Nil || seen[id] {
			return invalid()
		}
		seen[id] = true
		if err := dashboardaccess.RequireObject(ctx, q, actor, "dashboard", id); err != nil {
			return err
		}
		d, err := q.GetDashboard(ctx, db.GetDashboardParams{ID: id, EnterpriseID: p.EnterpriseID})
		if err != nil || d.Lifecycle != "active" || !d.ActiveRevisionID.Valid {
			return dashboardaccess.ErrDenied
		}
	}
	return nil
}
func Prepare(ctx context.Context, q *db.Queries, p toolruntime.Principal, input *Selection) (Prepared, error) {
	current, err := Current(ctx, q, p)
	if err != nil {
		return Prepared{}, err
	}
	result := Prepared{Snapshot: current, BaseVersion: current.Version}
	if input != nil {
		if input.ExpectedVersion != nil && *input.ExpectedVersion != current.Version {
			return result, conflict()
		}
		result.Snapshot.Mode = input.Mode
		result.Snapshot.DashboardIDs = append([]uuid.UUID{}, input.DashboardIDs...)
		result.Changed = input.Mode != current.Mode || !slices.Equal(input.DashboardIDs, current.DashboardIDs)
		if result.Changed {
			result.Snapshot.Version++
		}
	}
	return result, Validate(ctx, q, p, result.Snapshot)
}

// Caller holds the conversation lock; preflight is read-only and cannot commit
// an ID selection. Each accepted user message gets its own immutable record.
func Commit(ctx context.Context, q *db.Queries, p toolruntime.Principal, run uuid.UUID, prepared Prepared) error {
	current, err := Current(ctx, q, p)
	if err != nil {
		return err
	}
	if current.Version != prepared.BaseVersion {
		return conflict()
	}
	if err := Validate(ctx, q, p, prepared.Snapshot); err != nil {
		return err
	}
	raw, err := json.Marshal(prepared.Snapshot)
	if err != nil {
		return err
	}
	if prepared.Changed {
		_, err = q.SetConversationDashboardContext(ctx, db.SetConversationDashboardContextParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID, Version: prepared.Snapshot.Version, Selection: raw})
		if err != nil {
			return err
		}
	}
	_, err = q.CreateRunDashboardContext(ctx, db.CreateRunDashboardContextParams{RunID: run, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID, ContextVersion: prepared.Snapshot.Version, Selection: raw})
	if err != nil {
		return err
	}
	return freezeParameters(ctx, q, p, run, prepared.Snapshot)
}
func ForRun(ctx context.Context, q *db.Queries, p toolruntime.Principal, run uuid.UUID) (Snapshot, error) {
	row, err := q.GetRunDashboardContext(ctx, db.GetRunDashboardContextParams{RunID: run, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		// Only pre-existing general Runs may lack the extension. A foreign or
		// invented Run is not an escape hatch for the analysis restrictions.
		r, e := q.GetRun(ctx, db.GetRunParams{ID: run, EnterpriseID: p.EnterpriseID})
		if e != nil || r.ActorUserID != p.UserID || r.ConversationID != p.ConversationID {
			return Snapshot{}, dashboardaccess.ErrDenied
		}
		return None(), nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	var result Snapshot
	if json.Unmarshal(row.Selection, &result) != nil || result.Version != row.ContextVersion {
		return result, invalid()
	}
	return result, Validate(ctx, q, p, result)
}

type contextKey struct{}

func Command(content string) *Selection {
	words := strings.Fields(content)
	if len(words) > 0 && (words[0] == "/创建仪表盘" || words[0] == "/create-dashboard") {
		return &Selection{Mode: "create", DashboardIDs: []uuid.UUID{}}
	}
	return nil
}

func WithSnapshot(ctx context.Context, s Snapshot) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}
func FromContext(ctx context.Context) (Snapshot, bool) {
	s, ok := ctx.Value(contextKey{}).(Snapshot)
	return s, ok
}
func Facts(ctx context.Context, q *db.Queries, p toolruntime.Principal, s Snapshot, runs ...uuid.UUID) (map[string]any, error) {
	if err := Validate(ctx, q, p, s); err != nil {
		return nil, err
	}
	refs := []map[string]any{}
	for _, id := range s.DashboardIDs {
		d, err := q.GetDashboard(ctx, db.GetDashboardParams{ID: id, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return nil, err
		}
		run := uuid.Nil
		if len(runs) > 0 {
			run = runs[0]
		}
		conditions, e := parameterFact(ctx, q, p, id, run)
		if e != nil {
			return nil, e
		}
		refs = append(refs, map[string]any{"id": id, "name": d.Name, "conditions": conditions})
	}
	facts := map[string]any{"schema_version": s.Schema, "mode": s.Mode, "version": s.Version, "selected_dashboards": refs, "selection_required": s.Mode == "analyze" && len(refs) == 0}
	if len(runs) > 0 && runs[0] != uuid.Nil {
		budget, err := q.GetDashboardRunBudget(ctx, db.GetDashboardRunBudgetParams{RunID: runs[0], EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err == nil {
			facts["run_budget"] = map[string]any{"scan_bytes_remaining": budget.ScanRemaining, "result_bytes_remaining": budget.BytesRemaining, "rows_remaining": budget.RowsRemaining, "samples_remaining": budget.SamplesRemaining, "calls_remaining": budget.CallsRemaining}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return facts, nil
}
