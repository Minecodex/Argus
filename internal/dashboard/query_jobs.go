package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

const QuerySourcePrefix = "dashboard-query:"

type QueryObjectStore interface {
	PutFile(context.Context, string, io.Reader, int64, string) error
	ReadFile(context.Context, string, int64, int64) (io.ReadCloser, error)
}

// Delivery is an internal port. Model inputs never supply object keys, hashes,
// provenance, compiled queries or Workspace destinations.
type QueryDelivery interface {
	ImportQueryFile(context.Context, toolruntime.Principal, uuid.UUID, string, string, io.Reader, int64, string) (db.WorkspaceFile, error)
}
type QueryJobs struct {
	Runtime  Runtime
	Objects  QueryObjectStore
	Delivery QueryDelivery
}
type QueryJobInput struct {
	AnalysisContextID  uuid.UUID            `json:"analysis_context_id,omitempty"`
	ExpectedRevisionID uuid.UUID            `json:"expected_revision_id,omitempty"`
	DashboardID        uuid.UUID            `json:"dashboard_id"`
	RunID              uuid.UUID            `json:"run_id,omitempty"`
	Parameters         ExecutionInput       `json:"parameters"`
	Drilldown          *QueryDrilldownInput `json:"drilldown,omitempty"`
}
type QueryFileView struct {
	ID              uuid.UUID  `json:"id"`
	PanelID         string     `json:"panel_id"`
	TargetID        string     `json:"target_id"`
	Kind            string     `json:"kind"`
	Bytes           int64      `json:"bytes"`
	Hash            string     `json:"sha256"`
	SourceRef       string     `json:"source_ref"`
	Path            string     `json:"path"`
	WorkspaceID     *uuid.UUID `json:"workspace_id,omitempty"`
	WorkspaceFileID *uuid.UUID `json:"workspace_file_id,omitempty"`
}
type QueryManifest struct {
	AnalysisContextID    uuid.UUID              `json:"analysis_context_id,omitempty"`
	Schema               string                 `json:"schema_version"`
	JobID                uuid.UUID              `json:"job_id"`
	AttemptID            uuid.UUID              `json:"attempt_id"`
	Compiler             string                 `json:"compiler_version"`
	Execution            Execution              `json:"execution"`
	Files                []QueryFileView        `json:"files"`
	Complete             bool                   `json:"complete"`
	AnalysisStatus       string                 `json:"analysis_status"`
	Definition           Spec                   `json:"definition"`
	SubjectID            uuid.UUID              `json:"subject_id"`
	AuthorizationVersion int64                  `json:"authorization_version"`
	Requested            ExecutionInput         `json:"requested_parameters"`
	Drilldown            *QueryDrilldownContext `json:"drilldown,omitempty"`
}
type QueryJobView struct {
	ID          uuid.UUID       `json:"id"`
	DashboardID uuid.UUID       `json:"dashboard_id"`
	RevisionID  uuid.UUID       `json:"revision_id"`
	Status      string          `json:"status"`
	Version     int64           `json:"version"`
	ErrorCode   string          `json:"error_code,omitempty"`
	Manifest    *QueryManifest  `json:"manifest,omitempty"`
	Files       []QueryFileView `json:"files"`
}
type queryPlan struct {
	AnalysisContextID uuid.UUID              `json:"analysis_context_id,omitempty"`
	Compiler          string                 `json:"compiler"`
	Scope             Execution              `json:"scope"`
	Budget            queryBudget            `json:"budget"`
	Requested         ExecutionInput         `json:"requested"`
	Drilldown         *QueryDrilldownContext `json:"drilldown,omitempty"`
}
type queryBudget struct {
	Scan, Bytes            int64
	Rows, Samples, Pending int
}

func freezeBudget(b *executionBudget) queryBudget {
	return queryBudget{b.scan, b.bytes, b.rows, b.samples, b.pending}
}
func (b queryBudget) ledger() *executionBudget {
	return &executionBudget{scan: b.Scan, bytes: b.Bytes, rows: b.Rows, samples: b.Samples, pending: b.Pending}
}
func queryHash(value []byte) string { h := sha256.Sum256(value); return hex.EncodeToString(h[:]) }
func queryView(job db.DashboardQueryJob) (QueryJobView, error) {
	result := QueryJobView{ID: job.ID, DashboardID: job.DashboardID, RevisionID: job.RevisionID, Status: job.Status, Version: job.Version, ErrorCode: job.ErrorCode, Files: []QueryFileView{}}
	if len(job.Manifest) > 0 {
		if err := json.Unmarshal(job.Manifest, &result.Manifest); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (jobs QueryJobs) principal(ctx context.Context, actor Actor, conversation uuid.UUID) (toolruntime.Principal, error) {
	if jobs.Runtime.Store == nil || actor.SubjectType != "user" {
		return toolruntime.Principal{}, ErrDenied
	}
	q := jobs.Runtime.Store.Queries
	if err := authorize(ctx, q, actor, "telemetry.dashboard.read"); err != nil {
		return toolruntime.Principal{}, err
	}
	if err := authorize(ctx, q, actor, "workspace.use"); err != nil {
		return toolruntime.Principal{}, err
	}
	c, err := q.GetConversation(ctx, db.GetConversationParams{ID: conversation, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
	if err != nil || c.Status != "active" {
		return toolruntime.Principal{}, ErrDenied
	}
	user, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: actor.SubjectID, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		return toolruntime.Principal{}, err
	}
	permissions, err := q.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{UserID: user.ID, DepartmentID: user.DepartmentID, EnterpriseID: actor.EnterpriseID})
	return toolruntime.Principal{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ConversationID: conversation, AuthorizationVersion: actor.AuthorizationVersion, Permissions: permissions}, err
}
func (jobs QueryJobs) checkJob(ctx context.Context, actor Actor, conversation uuid.UUID, job db.DashboardQueryJob) error {
	if job.EnterpriseID != actor.EnterpriseID || job.OwnerUserID != actor.SubjectID || job.ConversationID != conversation {
		return ErrDenied
	}
	if _, err := jobs.principal(ctx, actor, conversation); err != nil {
		return err
	}
	item, _, err := (Service{Store: jobs.Runtime.Store}).Get(ctx, actor, job.DashboardID)
	if err != nil {
		return err
	}
	if item.Lifecycle != "active" {
		return ErrArchived
	}
	var plan queryPlan
	if json.Unmarshal(job.FrozenPlan, &plan) != nil {
		return ErrInvalid
	}
	if plan.Compiler != CompilerVersion {
		return ErrContextExpired
	}
	ids := make([]uuid.UUID, 0, len(plan.Scope.Resources))
	for _, r := range plan.Scope.Resources {
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 && len(plan.Scope.Panels) > 0 {
		return ErrDenied
	}
	if len(ids) > 0 {
		_, err = resolveResources(ctx, jobs.Runtime.Store.Queries, actor, ids)
	}
	return err
}

// Start is for a user-authorized request. Chat tool adapters must additionally
// bind dashboard selection to the immutable user-message/Run context.
func (jobs QueryJobs) Start(ctx context.Context, actor Actor, conversation uuid.UUID, input QueryJobInput, key string) (QueryJobView, error) {
	if jobs.Objects == nil || jobs.Runtime.Backend == nil {
		return QueryJobView{}, ErrUnavailable
	}
	if len(key) < 1 || len(key) > 160 || input.DashboardID == uuid.Nil || input.Parameters.CandidatesOnly {
		return QueryJobView{}, ErrInvalid
	}
	if _, err := jobs.principal(ctx, actor, conversation); err != nil {
		return QueryJobView{}, err
	}
	encoded, err := json.Marshal(input)
	if err != nil || len(encoded) > 1<<20 {
		return QueryJobView{}, ErrInvalid
	}
	// Own every map/slice before doing I/O; the frozen plan and idempotency hash
	// must describe the same request even for in-process callers.
	var owned QueryJobInput
	if err = json.Unmarshal(encoded, &owned); err != nil {
		return QueryJobView{}, ErrInvalid
	}
	input = owned
	actor.RunID = input.RunID
	digest := queryHash(encoded)
	lookup := db.FindDashboardQueryJobParams{EnterpriseID: actor.EnterpriseID, ConversationID: conversation, OwnerUserID: actor.SubjectID, RequestKey: key}
	replay := func(old db.DashboardQueryJob) (QueryJobView, error) {
		if old.InputHash != digest {
			return QueryJobView{}, ErrConflict
		}
		if err := jobs.checkJob(ctx, actor, conversation, old); err != nil {
			return QueryJobView{}, err
		}
		return queryView(old)
	}
	if old, err := jobs.Runtime.Store.Queries.FindDashboardQueryJob(ctx, lookup); err == nil {
		return replay(old)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return QueryJobView{}, err
	}
	if input.RunID != uuid.Nil {
		run, err := jobs.Runtime.Store.Queries.GetRun(ctx, db.GetRunParams{ID: input.RunID, EnterpriseID: actor.EnterpriseID})
		if err != nil || run.ActorUserID != actor.SubjectID || run.ConversationID != conversation || run.Status == "cancelled" {
			return QueryJobView{}, ErrDenied
		}
	}
	item, revision, err := (Service{Store: jobs.Runtime.Store}).Get(ctx, actor, input.DashboardID)
	if err != nil {
		return QueryJobView{}, err
	}
	if item.Lifecycle != "active" {
		return QueryJobView{}, ErrArchived
	}
	if input.AnalysisContextID != uuid.Nil {
		if input.Drilldown != nil || input.ExpectedRevisionID != revision.ID {
			return QueryJobView{}, ErrContextExpired
		}
		parameters, e := jobs.Runtime.Store.Queries.GetDashboardRunParameters(ctx, db.GetDashboardRunParametersParams{RunID: input.RunID, DashboardID: input.DashboardID, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
		resolution, e2 := jobs.Runtime.Store.Queries.GetDashboardAnalysisContext(ctx, db.GetDashboardAnalysisContextParams{ID: input.AnalysisContextID, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, RunID: input.RunID, OwnerUserID: actor.SubjectID})
		if e != nil || e2 != nil || parameters.Version != resolution.ConditionVersion {
			return QueryJobView{}, ErrContextExpired
		}
	}
	var plan queryPlan
	plan.AnalysisContextID = input.AnalysisContextID
	if input.Drilldown != nil {
		plan, err = jobs.prepareFileDrilldown(ctx, actor, conversation, input)
	} else {
		var spec Spec
		spec, err = DecodeSpec(revision.Spec)
		if err == nil {
			var budget *executionBudget
			plan.Scope, budget, err = jobs.Runtime.prepareSpec(ctx, actor, spec, input.Parameters)
			if err == nil {
				plan.Budget = freezeBudget(budget)
			}
		}
		plan.Scope.RevisionID = revision.ID
		plan.Requested = input.Parameters
	}
	if err != nil {
		if errors.Is(err, errNoResources) {
			err = ErrDenied
		}
		return QueryJobView{}, err
	}
	plan.Compiler = CompilerVersion
	plan.Scope.DashboardID, plan.Scope.ExecutionID = input.DashboardID, uuid.New()
	identity, err := json.Marshal(struct {
		Scope     Execution
		Drilldown *QueryDrilldownContext
	}{executionIdentity(plan.Scope), plan.Drilldown})
	if err != nil {
		return QueryJobView{}, err
	}
	plan.Scope.ExecutionHash = queryHash(identity)
	frozen, err := json.Marshal(plan)
	if err != nil || len(frozen) > 2<<20 {
		return QueryJobView{}, ErrInvalid
	}
	var job db.DashboardQueryJob
	err = jobs.Runtime.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		// Serializing against the owning conversation also excludes delete and
		// duplicate enqueues. Source authorization is checked again after preparation.
		c, e := q.LockConversation(ctx, db.LockConversationParams{ID: conversation, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
		if e != nil {
			return e
		}
		if c.Status != "active" {
			return ErrDenied
		}
		if old, e := q.FindDashboardQueryJob(ctx, lookup); e == nil {
			if old.InputHash != digest {
				return ErrConflict
			}
			job = old
			return nil
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		candidate := db.DashboardQueryJob{EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID, ConversationID: conversation, DashboardID: item.ID, FrozenPlan: frozen}
		if e := jobs.checkJob(ctx, actor, conversation, candidate); e != nil {
			return e
		}
		if e := recordAnalysisEffective(ctx, q, actor, conversation, input, plan.Scope); e != nil {
			return e
		}
		id, taskID := uuid.New(), uuid.New()
		payload, _ := json.Marshal(map[string]any{"job_id": id})
		if _, e = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: taskID, EnterpriseID: nullID(actor.EnterpriseID), Queue: "dashboard_query", RunID: nullID(input.RunID), Payload: payload, MaxAttempts: 5, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}); e != nil {
			return e
		}
		job, e = q.CreateDashboardQueryJob(ctx, db.CreateDashboardQueryJobParams{ID: id, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, OwnerUserID: actor.SubjectID, RunID: nullID(input.RunID), DashboardID: item.ID, RevisionID: plan.Scope.RevisionID, AuthorizationVersion: actor.AuthorizationVersion, TaskID: taskID, RequestKey: key, InputHash: digest, FrozenPlan: frozen})
		if e != nil {
			return e
		}
		return recordQueryJob(ctx, q, actor, job, "dashboard.query.queued", "")
	})
	if err != nil {
		return QueryJobView{}, err
	}
	return queryView(job)
}
func (jobs QueryJobs) loadJob(ctx context.Context, actor Actor, conversation, id uuid.UUID) (db.DashboardQueryJob, error) {
	job, err := jobs.Runtime.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: id, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		return job, ErrNotFound
	}
	if err = jobs.checkJob(ctx, actor, conversation, job); err != nil {
		return job, err
	}
	return job, nil
}
func (jobs QueryJobs) Get(ctx context.Context, actor Actor, conversation, id uuid.UUID) (QueryJobView, error) {
	job, err := jobs.loadJob(ctx, actor, conversation, id)
	if err != nil {
		return QueryJobView{}, err
	}
	view, err := queryView(job)
	if err != nil {
		return view, err
	}
	if len(job.Manifest) > 0 && job.AttemptID.Valid {
		files, e := jobs.Runtime.Store.Queries.ListDashboardQueryFiles(ctx, db.ListDashboardQueryFilesParams{AttemptID: job.AttemptID.UUID, EnterpriseID: job.EnterpriseID})
		if e != nil {
			return view, e
		}
		for _, f := range files {
			view.Files = append(view.Files, queryFileView(f))
		}
		workspace, e := jobs.Runtime.Store.Queries.GetConversationWorkspace(ctx, db.GetConversationWorkspaceParams{ConversationID: conversation, EnterpriseID: actor.EnterpriseID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return view, e
		}
		if e == nil && workspace.Status == "ready" {
			for i, f := range files {
				delivery, e := jobs.Runtime.Store.Queries.GetDashboardQueryDelivery(ctx, db.GetDashboardQueryDeliveryParams{FileID: f.ID, WorkspaceID: workspace.ID, EnterpriseID: actor.EnterpriseID})
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return view, e
				}
				if e == nil {
					view.Files[i].WorkspaceID = &delivery.WorkspaceID
					view.Files[i].WorkspaceFileID = &delivery.WorkspaceFileID
				}
			}
		}
	}
	return view, nil
}
func (jobs QueryJobs) Cancel(ctx context.Context, actor Actor, conversation, id uuid.UUID) (QueryJobView, error) {
	if _, err := jobs.loadJob(ctx, actor, conversation, id); err != nil {
		return QueryJobView{}, err
	}
	var job db.DashboardQueryJob
	err := jobs.Runtime.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		var err error
		job, err = q.LockDashboardQueryJob(ctx, db.LockDashboardQueryJobParams{ID: id, EnterpriseID: actor.EnterpriseID})
		if err != nil {
			return err
		}
		if terminalQueryJob(job.Status) {
			return nil
		}
		job, err = q.SetDashboardQueryJobState(ctx, db.SetDashboardQueryJobStateParams{ID: id, EnterpriseID: actor.EnterpriseID, Status: "cancelled", AttemptID: job.AttemptID, ErrorCode: "QUERY_CANCELLED"})
		if err != nil {
			return err
		}
		return recordQueryJob(ctx, q, actor, job, "dashboard.query.cancelled", "")
	})
	if err != nil {
		return QueryJobView{}, err
	}
	return queryView(job)
}
func terminalQueryJob(status string) bool {
	return status == "complete" || status == "partial" || status == "cancelled" || status == "failed"
}

func (jobs QueryJobs) Resume(ctx context.Context, actor Actor, conversation, id uuid.UUID, version int64) (QueryJobView, error) {
	original, err := jobs.loadJob(ctx, actor, conversation, id)
	if err != nil {
		return QueryJobView{}, err
	}
	var job db.DashboardQueryJob
	err = jobs.Runtime.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		// Match the Worker's lock order: runtime task, then query job.
		n, e := q.RetryDashboardQueryTask(ctx, db.RetryDashboardQueryTaskParams{ID: original.TaskID, EnterpriseID: nullID(original.EnterpriseID)})
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrConflict
		}
		var err error
		job, err = q.LockDashboardQueryJob(ctx, db.LockDashboardQueryJobParams{ID: id, EnterpriseID: actor.EnterpriseID})
		if err != nil {
			return err
		}
		if job.Version != version || job.Status != "failed" {
			return ErrConflict
		}
		state := "queued"
		if len(job.Manifest) > 0 {
			state = "materialized"
		}
		job, err = q.SetDashboardQueryJobState(ctx, db.SetDashboardQueryJobStateParams{ID: id, EnterpriseID: job.EnterpriseID, Status: state, AttemptID: job.AttemptID})
		if err != nil {
			return err
		}
		return recordQueryJob(ctx, q, actor, job, "dashboard.query.resumed", "")
	})
	if err != nil {
		return QueryJobView{}, err
	}
	return queryView(job)
}

// AuthorizeSource is called for every Workspace read, write, download and
// computation, including derived files in a directory containing this source.
func (jobs QueryJobs) AuthorizeSource(ctx context.Context, p toolruntime.Principal, ref string) (bool, error) {
	if !strings.HasPrefix(ref, QuerySourcePrefix) {
		return false, nil
	}
	jobText, attemptText, ok := strings.Cut(strings.TrimPrefix(ref, QuerySourcePrefix), "/")
	if !ok {
		return true, ErrDenied
	}
	id, err := uuid.Parse(jobText)
	if err != nil {
		return true, ErrDenied
	}
	attempt, err := uuid.Parse(attemptText)
	if err != nil {
		return true, ErrDenied
	}
	job, err := jobs.Runtime.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return true, ErrDenied
	}
	if !job.AttemptID.Valid || attempt != job.AttemptID.UUID || len(job.Manifest) == 0 {
		return true, ErrDenied
	}
	return true, jobs.checkJob(ctx, Actor{EnterpriseID: p.EnterpriseID, SubjectID: p.UserID, SubjectType: "user", AuthorizationVersion: p.AuthorizationVersion}, p.ConversationID, job)
}
