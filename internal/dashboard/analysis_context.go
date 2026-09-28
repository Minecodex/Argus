package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"reflect"
	"slices"
	"time"
)

type AnalysisContextView struct {
	ID               uuid.UUID             `json:"context_ref"`
	DashboardID      uuid.UUID             `json:"dashboard_id"`
	RevisionID       uuid.UUID             `json:"revision_id"`
	ConditionVersion int64                 `json:"condition_version"`
	Conditions       dashboardparams.State `json:"conditions"`
	Parameters       ExecutionInput        `json:"parameters"`
}
type AnalysisQueryInput struct {
	DashboardID uuid.UUID            `json:"dashboard_id"`
	ContextRef  uuid.UUID            `json:"context_ref,omitempty"`
	PanelIDs    []string             `json:"panel_ids,omitempty"`
	Drilldown   *QueryDrilldownInput `json:"drilldown,omitempty"`
}

func analysisPrincipal(actor Actor, conversation uuid.UUID) toolruntime.Principal {
	return toolruntime.Principal{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ConversationID: conversation, AuthorizationVersion: actor.AuthorizationVersion}
}
func analysisScope(ctx context.Context, q *db.Queries, actor Actor, conversation, dashboard uuid.UUID, active bool) error {
	if actor.SubjectType != "user" || actor.RunID == uuid.Nil {
		return ErrDenied
	}
	selection, err := dashboardcontext.ForRun(ctx, q, analysisPrincipal(actor, conversation), actor.RunID)
	if err != nil {
		return err
	}
	if !slices.Contains(selection.DashboardIDs, dashboard) {
		return toolruntime.Error{Kind: "DASHBOARD_SELECTION_REQUIRED"}
	}
	run, err := q.GetRun(ctx, db.GetRunParams{ID: actor.RunID, EnterpriseID: actor.EnterpriseID})
	if err != nil || run.ConversationID != conversation || run.ActorUserID != actor.SubjectID || active && !slices.Contains([]string{"pending", "running", "waiting_input", "waiting_approval", "waiting_system"}, run.Status) {
		return ErrContextExpired
	}
	return nil
}
func analysisView(row db.DashboardAnalysisContext) (AnalysisContextView, error) {
	state, err := decodeConditions(row.State)
	if err != nil {
		return AnalysisContextView{}, err
	}
	result := AnalysisContextView{ID: row.ID, DashboardID: row.DashboardID, RevisionID: row.RevisionID, ConditionVersion: row.ConditionVersion, Conditions: state}
	err = json.Unmarshal(row.Parameters, &result.Parameters)
	return result, err
}
func (service Service) InspectAnalysis(ctx context.Context, actor Actor, conversation, id uuid.UUID) (map[string]any, error) {
	q := service.Store.Queries
	if err := analysisScope(ctx, q, actor, conversation, id, false); err != nil {
		return nil, err
	}
	_, revision, err := service.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return nil, err
	}
	row, err := q.GetDashboardRunParameters(ctx, db.GetDashboardRunParametersParams{RunID: actor.RunID, DashboardID: id, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
	if err != nil {
		return nil, ErrContextExpired
	}
	state, err := decodeConditions(row.State)
	if err != nil {
		return nil, err
	}
	enterprise, err := q.GetEnterprise(ctx, actor.EnterpriseID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"condition_version": row.Version, "conditions": state, "incompatible_paths": inheritedConflicts(state, conditionContracts(spec)), "revision_id": revision.ID, "server_time": time.Now().UTC(), "timezone": enterprise.Timezone}, nil
}

// ResolveAnalysis saves only message-attributed overrides. The immutable context
// materializes their time range; model strategy (which panels to check) is not
// persisted as an explicit user condition.
func (service Service) ResolveAnalysis(ctx context.Context, actor Actor, conversation uuid.UUID, input AnalysisResolveInput, invocation uuid.UUID) (AnalysisContextView, error) {
	var result AnalysisContextView
	if invocation == uuid.Nil || input.ExpectedVersion < 0 {
		return result, ErrInvalid
	}
	encoded, err := json.Marshal(input)
	if err != nil || len(encoded) > 128<<10 {
		return result, ErrInvalid
	}
	digest := queryHash(encoded)
	var owned AnalysisResolveInput
	if json.Unmarshal(encoded, &owned) != nil {
		return result, ErrInvalid
	}
	input = owned
	id := uuid.NewSHA1(actor.RunID, []byte("analysis:"+invocation.String()))
	q := service.Store.Queries
	if err = analysisScope(ctx, q, actor, conversation, input.DashboardID, true); err != nil {
		return result, err
	}
	lookup := db.GetDashboardAnalysisContextParams{ID: id, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, RunID: actor.RunID, OwnerUserID: actor.SubjectID}
	replay := func(row db.DashboardAnalysisContext) (AnalysisContextView, error) {
		if row.InputHash != digest {
			return result, ErrConflict
		}
		return analysisView(row)
	}
	if row, e := q.GetDashboardAnalysisContext(ctx, lookup); e == nil {
		return replay(row)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return result, e
	}
	err = service.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		c, e := q.LockConversation(ctx, db.LockConversationParams{ID: conversation, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
		if e != nil || c.Status != "active" {
			return ErrDenied
		}
		if e = analysisScope(ctx, q, actor, conversation, input.DashboardID, true); e != nil {
			return e
		}
		if row, e := q.GetDashboardAnalysisContext(ctx, lookup); e == nil {
			result, e = replay(row)
			return e
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		row, e := q.LockDashboardRunParameters(ctx, db.LockDashboardRunParametersParams{RunID: actor.RunID, DashboardID: input.DashboardID, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
		if e != nil {
			return ErrContextExpired
		}
		if row.Version != input.ExpectedVersion {
			return ErrConflict
		}
		old, e := decodeConditions(row.State)
		if e != nil {
			return e
		}
		item, e := q.GetDashboard(ctx, db.GetDashboardParams{ID: input.DashboardID, EnterpriseID: actor.EnterpriseID})
		if e != nil || item.Lifecycle != "active" {
			return ErrArchived
		}
		revision, e := q.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: item.ActiveRevisionID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			return e
		}
		spec, e := DecodeSpec(revision.Spec)
		if e != nil {
			return e
		}
		message, e := q.GetDashboardRunUserMessage(ctx, db.GetDashboardRunUserMessageParams{RunID: nullID(actor.RunID), EnterpriseID: actor.EnterpriseID, ConversationID: conversation})
		if e != nil {
			return ErrContextExpired
		}
		var payload struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(message.Payload, &payload) != nil || message.ActorType != "user" || (!message.ActorID.Valid || message.ActorID.String != actor.SubjectID.String()) {
			return ErrDenied
		}
		state, e := mergeConditions(old, input.Changes, input.Evidence, message.ID, payload.Content, conditionContracts(spec))
		if e != nil {
			return e
		}
		parameters, e := conditionParameters(spec, state, time.Now().UTC())
		if e != nil {
			return e
		}
		if len(parameters.ResourceIDs) > 0 {
			if _, e = resolveResources(ctx, q, actor, parameters.ResourceIDs); e != nil {
				return e
			}
		}
		version := row.Version
		if !reflect.DeepEqual(old, state) {
			version, e = saveAnalysisState(ctx, q, actor, conversation, input.DashboardID, version, state)
			if e != nil {
				return e
			}
		}
		raw, _ := json.Marshal(state)
		params, _ := json.Marshal(parameters)
		saved, e := q.CreateDashboardAnalysisContext(ctx, db.CreateDashboardAnalysisContextParams{ID: id, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, RunID: actor.RunID, DashboardID: input.DashboardID, OwnerUserID: actor.SubjectID, RevisionID: revision.ID, ConditionVersion: version, InputHash: digest, UserEventID: message.ID, State: raw, Parameters: params})
		if e != nil {
			return e
		}
		result, e = analysisView(saved)
		return e
	})
	return result, translateConflict(err)
}

func saveAnalysisState(ctx context.Context, q *db.Queries, actor Actor, conversation, id uuid.UUID, version int64, state dashboardparams.State) (int64, error) {
	raw, err := json.Marshal(state)
	if err != nil || len(raw) > 64<<10 {
		return version, ErrInvalid
	}
	next, err := q.SetDashboardRunParameters(ctx, db.SetDashboardRunParametersParams{RunID: actor.RunID, DashboardID: id, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID, ExpectedVersion: version, State: raw})
	if err != nil {
		return version, ErrConflict
	}
	_, err = q.SetDashboardParameterState(ctx, db.SetDashboardParameterStateParams{EnterpriseID: actor.EnterpriseID, ConversationID: conversation, DashboardID: id, OwnerUserID: actor.SubjectID, RunID: actor.RunID, ExpectedVersion: version, Version: next.Version, State: raw})
	return next.Version, err
}
func (jobs QueryJobs) StartAnalysis(ctx context.Context, actor Actor, conversation uuid.UUID, input AnalysisQueryInput, key string) (QueryJobView, error) {
	if err := analysisScope(ctx, jobs.Runtime.Store.Queries, actor, conversation, input.DashboardID, true); err != nil {
		return QueryJobView{}, err
	}
	request := QueryJobInput{DashboardID: input.DashboardID, RunID: actor.RunID, Drilldown: input.Drilldown}
	if input.Drilldown != nil {
		if input.ContextRef != uuid.Nil || len(input.PanelIDs) > 0 {
			return QueryJobView{}, ErrInvalid
		}
	} else {
		if input.ContextRef == uuid.Nil {
			return QueryJobView{}, ErrContextExpired
		}
		row, err := jobs.Runtime.Store.Queries.GetDashboardAnalysisContext(ctx, db.GetDashboardAnalysisContextParams{ID: input.ContextRef, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, RunID: actor.RunID, OwnerUserID: actor.SubjectID})
		if err != nil || row.DashboardID != input.DashboardID {
			return QueryJobView{}, ErrContextExpired
		}
		view, err := analysisView(row)
		if err != nil {
			return QueryJobView{}, err
		}
		request.Parameters = view.Parameters
		request.Parameters.PanelIDs = input.PanelIDs
		request.AnalysisContextID = row.ID
		request.ExpectedRevisionID = row.RevisionID
	}
	return jobs.Start(ctx, actor, conversation, request, key)
}

// Candidate reconciliation changes only explicit filter values, never default
// values or the resource authorization scope. A newer condition wins over an
// older in-flight preparation; the older execution retains its own manifest.
func recordAnalysisEffective(ctx context.Context, q *db.Queries, actor Actor, conversation uuid.UUID, input QueryJobInput, scope Execution) error {
	if input.AnalysisContextID == uuid.Nil {
		return nil
	}
	row, err := q.GetDashboardAnalysisContext(ctx, db.GetDashboardAnalysisContextParams{ID: input.AnalysisContextID, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, RunID: input.RunID, OwnerUserID: actor.SubjectID})
	if err != nil {
		return ErrContextExpired
	}
	state, err := decodeConditions(row.State)
	if err != nil {
		return err
	}
	state, changed := reconcileExplicit(state, scope)
	if !changed {
		return nil
	}
	current, err := q.LockDashboardRunParameters(ctx, db.LockDashboardRunParametersParams{RunID: input.RunID, DashboardID: input.DashboardID, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
	if err != nil {
		return err
	}
	if current.Version != row.ConditionVersion {
		return nil
	}
	actor.RunID = input.RunID
	_, err = saveAnalysisState(ctx, q, actor, conversation, input.DashboardID, current.Version, state)
	return err
}
