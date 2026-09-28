package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type queryTask struct {
	JobID uuid.UUID `json:"job_id"`
}
type queryWork struct {
	jobs  QueryJobs
	task  runtime.Task
	actor Actor
	job   db.DashboardQueryJob
}

func (jobs QueryJobs) Handle(ctx context.Context, task runtime.Task) error {
	if !task.EnterpriseID.Valid || task.Queue != "dashboard_query" {
		return runtime.Error{ErrorCode: "QUERY_INVALID", Permanent: true}
	}
	var payload queryTask
	if json.Unmarshal(task.Payload, &payload) != nil || payload.JobID == uuid.Nil {
		return runtime.Error{ErrorCode: "QUERY_INVALID", Permanent: true}
	}
	job, err := jobs.Runtime.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: payload.JobID, EnterpriseID: task.EnterpriseID.UUID})
	if err != nil {
		return err
	}
	if job.TaskID != task.ID {
		return runtime.Error{ErrorCode: "QUERY_INVALID", Permanent: true}
	}
	if terminalQueryJob(job.Status) {
		return nil
	}
	user, err := jobs.Runtime.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: job.OwnerUserID, EnterpriseID: job.EnterpriseID})
	if err != nil {
		return err
	}
	work := queryWork{jobs, task, Actor{RunID: job.RunID.UUID, EnterpriseID: job.EnterpriseID, SubjectID: job.OwnerUserID, SubjectType: "user", AuthorizationVersion: user.AuthorizationVersion}, job}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	done := make(chan struct{})
	monitor := work
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if monitor.guard(ctx) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	err = work.run(ctx)
	if err == nil {
		return nil
	}
	cleanup, stopCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stopCleanup()
	current, e := jobs.Runtime.Store.Queries.GetDashboardQueryJob(cleanup, db.GetDashboardQueryJobParams{ID: job.ID, EnterpriseID: job.EnterpriseID})
	if e == nil && current.Status == "cancelled" {
		return nil
	}
	if job.RunID.Valid {
		run, e := jobs.Runtime.Store.Queries.GetRun(cleanup, db.GetRunParams{ID: job.RunID.UUID, EnterpriseID: job.EnterpriseID})
		if e == nil && run.Status == "cancelled" {
			return work.transaction(cleanup, func(q *db.Queries, j db.DashboardQueryJob) error {
				_, e := q.SetDashboardQueryJobState(cleanup, db.SetDashboardQueryJobStateParams{ID: j.ID, EnterpriseID: j.EnterpriseID, Status: "cancelled", AttemptID: j.AttemptID, ErrorCode: "RUN_CANCELLED"})
				return e
			})
		}
	}
	code, permanent := "DASHBOARD_QUERY_UNAVAILABLE", false
	if errors.Is(err, ErrDenied) || errors.Is(err, ErrArchived) {
		code, permanent = "DASHBOARD_DENIED", true
	}
	if errors.Is(err, ErrInvalid) {
		code, permanent = "DASHBOARD_INVALID", true
	}
	if te, ok := err.(toolruntime.Error); ok {
		code = te.Kind
	}
	if permanent {
		_ = work.fail(cleanup, code)
	} else {
		_ = work.transaction(cleanup, func(q *db.Queries, j db.DashboardQueryJob) error {
			_, e := q.SetDashboardQueryJobState(cleanup, db.SetDashboardQueryJobStateParams{ID: j.ID, EnterpriseID: j.EnterpriseID, Status: j.Status, AttemptID: j.AttemptID, ErrorCode: code})
			return e
		})
	}
	return runtime.Error{ErrorCode: code, Cause: err, Permanent: permanent}
}
func (w queryWork) guard(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	q := w.jobs.Runtime.Store.Queries
	valid, err := q.IsRuntimeTaskLeaseCurrent(ctx, db.IsRuntimeTaskLeaseCurrentParams{ID: w.task.ID, LeaseOwner: w.task.LeaseOwner, FenceToken: w.task.FenceToken})
	if err != nil {
		return err
	}
	if !valid {
		return context.Canceled
	}
	job, err := q.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: w.job.ID, EnterpriseID: w.job.EnterpriseID})
	if err != nil {
		return err
	}
	if terminalQueryJob(job.Status) {
		return context.Canceled
	}
	if job.RunID.Valid {
		run, e := q.GetRun(ctx, db.GetRunParams{ID: job.RunID.UUID, EnterpriseID: job.EnterpriseID})
		if e != nil {
			return e
		}
		if run.Status == "cancelled" {
			return context.Canceled
		}
	}
	return w.jobs.checkJob(ctx, w.actor, job.ConversationID, job)
}
func (w queryWork) transaction(ctx context.Context, fn func(*db.Queries, db.DashboardQueryJob) error) error {
	return w.jobs.Runtime.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockRuntimeTaskLease(ctx, db.LockRuntimeTaskLeaseParams{ID: w.task.ID, LeaseOwner: w.task.LeaseOwner, FenceToken: w.task.FenceToken}); err != nil {
			return err
		}
		job, err := q.LockDashboardQueryJob(ctx, db.LockDashboardQueryJobParams{ID: w.job.ID, EnterpriseID: w.job.EnterpriseID})
		if err != nil {
			return err
		}
		if terminalQueryJob(job.Status) {
			return context.Canceled
		}
		return fn(q, job)
	})
}
func (w queryWork) fail(ctx context.Context, code string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
		updated, err := q.SetDashboardQueryJobState(ctx, db.SetDashboardQueryJobStateParams{ID: job.ID, EnterpriseID: job.EnterpriseID, Status: "failed", AttemptID: job.AttemptID, ErrorCode: code})
		if err != nil {
			return err
		}
		return recordQueryJob(ctx, q, w.actor, updated, "dashboard.query.failed", code)
	})
}
func (jobs QueryJobs) HandleExhausted(ctx context.Context, task runtime.Task, cause error) error {
	var payload queryTask
	if json.Unmarshal(task.Payload, &payload) != nil {
		return nil
	}
	job, err := jobs.Runtime.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: payload.JobID, EnterpriseID: task.EnterpriseID.UUID})
	if err != nil {
		return err
	}
	if terminalQueryJob(job.Status) {
		return nil
	}
	code := "DASHBOARD_QUERY_RETRY_EXHAUSTED"
	var coded interface{ Code() string }
	if errors.As(cause, &coded) && coded.Code() != "" {
		code = coded.Code()
	}
	return (queryWork{jobs: jobs, task: task, job: job}).fail(ctx, code)
}
func (w *queryWork) run(ctx context.Context) error {
	if err := w.guard(ctx); err != nil {
		return err
	}
	if len(w.job.Manifest) == 0 {
		if err := w.fetch(ctx); err != nil {
			return err
		}
	}
	if err := w.checkpoint(ctx, "materialized"); err != nil {
		return err
	}
	return w.deliver(ctx)
}
func (w *queryWork) fetch(ctx context.Context) error {
	var plan queryPlan
	if json.Unmarshal(w.job.FrozenPlan, &plan) != nil || plan.Compiler != CompilerVersion {
		return ErrInvalid
	}
	revision, err := w.jobs.Runtime.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: w.job.RevisionID, EnterpriseID: w.job.EnterpriseID})
	if err != nil {
		return err
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return err
	}
	err = w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
		if len(job.Manifest) > 0 {
			w.job = job
			return nil
		}
		if err := q.AbandonDashboardQueryAttempts(ctx, db.AbandonDashboardQueryAttemptsParams{JobID: job.ID, EnterpriseID: job.EnterpriseID}); err != nil {
			return err
		}
		attempt, err := q.CreateDashboardQueryAttempt(ctx, db.CreateDashboardQueryAttemptParams{ID: uuid.New(), EnterpriseID: job.EnterpriseID, JobID: job.ID, Ordinal: w.task.Attempt})
		if err != nil {
			return err
		}
		w.job, err = q.SetDashboardQueryJobState(ctx, db.SetDashboardQueryJobStateParams{ID: job.ID, EnterpriseID: job.EnterpriseID, Status: "fetching", AttemptID: nullID(attempt.ID)})
		return err
	})
	if err != nil {
		return err
	}
	if len(w.job.Manifest) > 0 {
		return nil
	}
	queryCtx, cancel := context.WithTimeout(withQueryCheckpoint(ctx, w.checkpoint), time.Minute)
	var result Execution
	if plan.Drilldown != nil {
		result, err = w.jobs.Runtime.executeFileDrilldown(queryCtx, w.actor, spec, plan)
	} else {
		result, err = w.jobs.Runtime.executePrepared(queryCtx, w.actor, spec, plan.Scope, plan.Budget.ledger())
	}
	cancel()
	if err != nil {
		return err
	}
	if err := w.guard(ctx); err != nil {
		return err
	}
	manifest := QueryManifest{Schema: "argus.dashboard_query_manifest/v1", JobID: w.job.ID, AttemptID: w.job.AttemptID.UUID, Compiler: CompilerVersion, Execution: result, Files: []QueryFileView{}, Complete: !result.Partial, AnalysisStatus: "not_analyzed"}
	manifest.Definition = spec
	manifest.AnalysisContextID = plan.AnalysisContextID
	manifest.Drilldown = plan.Drilldown
	manifest.SubjectID, manifest.AuthorizationVersion, manifest.Requested = w.actor.SubjectID, w.actor.AuthorizationVersion, plan.Requested
	for pi := range result.Panels {
		for ti := range result.Panels[pi].Targets {
			target := &manifest.Execution.Panels[pi].Targets[ti]
			if target.Data != nil {
				file, err := w.jobs.materializeFile(ctx, w.job, result.Panels[pi].ID, target.ID, "data", target.Data, func() error { return w.guard(ctx) })
				if err != nil {
					return err
				}
				if err = w.storeFile(ctx, file); err != nil {
					return err
				}
				manifest.Files = append(manifest.Files, queryFileView(file))
			}
			target.Data = nil
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	index, err := w.jobs.materializeFile(ctx, w.job, "", "", "manifest", manifest, func() error { return w.guard(ctx) })
	if err != nil {
		return err
	}
	if err = w.storeFile(ctx, index); err != nil {
		return err
	}
	return w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
		if job.AttemptID != w.job.AttemptID {
			return ErrConflict
		}
		if _, err := q.SealDashboardQueryAttempt(ctx, db.SealDashboardQueryAttemptParams{ID: job.AttemptID.UUID, EnterpriseID: job.EnterpriseID}); err != nil {
			return err
		}
		var err error
		w.job, err = q.SealDashboardQueryJob(ctx, db.SealDashboardQueryJobParams{ID: job.ID, EnterpriseID: job.EnterpriseID, Manifest: raw, AttemptID: job.AttemptID})
		if err != nil {
			return err
		}
		return recordQueryJob(ctx, q, w.actor, w.job, "dashboard.query.materialized", "")
	})
}
func (w queryWork) storeFile(ctx context.Context, file db.DashboardQueryFile) error {
	return w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
		if job.AttemptID.UUID != file.AttemptID || job.Status != "fetching" {
			return ErrConflict
		}
		_, err := q.CreateDashboardQueryFile(ctx, db.CreateDashboardQueryFileParams{ID: file.ID, EnterpriseID: file.EnterpriseID, JobID: file.JobID, AttemptID: file.AttemptID, PanelID: file.PanelID, TargetID: file.TargetID, FileKind: file.FileKind, ByteSize: file.ByteSize, ContentHash: file.ContentHash, Chunks: file.Chunks})
		return err
	})
}
func (w *queryWork) deliver(ctx context.Context) error {
	if w.jobs.Delivery == nil {
		return ErrUnavailable
	}
	if err := w.guard(ctx); err != nil {
		return err
	}
	principal, err := w.jobs.principal(ctx, w.actor, w.job.ConversationID)
	if err != nil {
		return err
	}
	files, err := w.jobs.Runtime.Store.Queries.ListDashboardQueryFiles(ctx, db.ListDashboardQueryFilesParams{AttemptID: w.job.AttemptID.UUID, EnterpriseID: w.job.EnterpriseID})
	if err != nil {
		return err
	}
	err = w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
		var err error
		w.job, err = q.SetDashboardQueryJobState(ctx, db.SetDashboardQueryJobStateParams{ID: job.ID, EnterpriseID: job.EnterpriseID, Status: "delivering", AttemptID: job.AttemptID})
		return err
	})
	if err != nil {
		return err
	}
	for _, file := range files {
		reader, err := newQueryFileReader(ctx, w.jobs.Objects, file, func() error { return w.guard(ctx) })
		if err != nil {
			return err
		}
		path := queryFilePath(file)
		delivered, err := w.jobs.Delivery.ImportQueryFile(ctx, principal, w.job.RunID.UUID, querySourceRef(file.JobID, file.AttemptID), path, reader, file.ByteSize, file.ContentHash)
		if err != nil {
			return err
		}
		if err = w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
			return q.CreateDashboardQueryDelivery(ctx, db.CreateDashboardQueryDeliveryParams{FileID: file.ID, EnterpriseID: file.EnterpriseID, WorkspaceID: delivered.WorkspaceID, WorkspaceFileID: delivered.ID})
		}); err != nil {
			return err
		}
		if err := w.checkpoint(ctx, "delivered_file"); err != nil {
			return err
		}
	}
	var manifest QueryManifest
	if json.Unmarshal(w.job.Manifest, &manifest) != nil {
		return ErrInvalid
	}
	status := "complete"
	if !manifest.Complete {
		status = "partial"
	}
	return w.transaction(ctx, func(q *db.Queries, job db.DashboardQueryJob) error {
		updated, err := q.SetDashboardQueryJobState(ctx, db.SetDashboardQueryJobStateParams{ID: job.ID, EnterpriseID: job.EnterpriseID, Status: status, AttemptID: job.AttemptID})
		if err != nil {
			return err
		}
		return recordQueryJob(ctx, q, w.actor, updated, "dashboard.query.delivered", "")
	})
}
