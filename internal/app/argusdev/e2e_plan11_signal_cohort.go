package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) plan11DeployedSignals(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if !plan11SignalPhase(env.Options.Plan8Phase) || env.Options.RealModel == nil || host == nil {
		return errors.New("signal cohort requires its explicit owned real-model phase")
	}
	d := plan11SignalDataset{Host: host.HostID, Dashboard: host.DashboardID, Application: env.State.Values["p11_signal_application_id"], Cluster: env.State.Values["p11_signal_cluster_id"], Namespace: env.State.Values["p11_signal_namespace_id"], OperationalMarker: env.State.Values["p11_signal_operational_marker"]}
	for _, id := range []string{d.Host, d.Dashboard, d.Application, d.Cluster, d.Namespace} {
		if uuid.Validate(id) != nil {
			return errors.New("signal cohort has incomplete actual resource prerequisites")
		}
	}
	if d.OperationalMarker == "" {
		return errors.New("actual operational log marker missing")
	}
	d.To = time.Now().UTC().Truncate(time.Second)
	d.From = d.To.Add(-30 * time.Minute)
	tasks, err := plan11SelectedSignalTasks(d, env.Options.Plan11SignalTasks)
	if err != nil {
		return err
	}
	bound := len(tasks) * 32
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		names = append(names, task.Name)
	}
	proof := map[string]any{"schema_version": "argus.plan11_signal_cohort/v1", "planned_initial_tasks": len(tasks), "selected_task_names": names, "full_nine_task_cohort": len(tasks) == 9, "provider_acceptance_bound": bound, "probes_separate": true, "dataset": d, "whole_plan_complete": false, "full_quality_requires_independent_review": true}
	declaration, _ := json.MarshalIndent(proof, "", "  ")
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "plan11-signal-admission-before-models.json"), declaration); err != nil {
		return err
	}
	cases := []map[string]any{}
	defer func() {
		calls := 0
		usageComplete := true
		for _, row := range cases {
			if sample, ok := row["sample"].(p5BenchmarkSample); ok {
				calls += sample.ModelCalls
				usageComplete = usageComplete && sample.UsageComplete && sample.CachedUsageComplete
			}
		}
		if calls > bound {
			failure = errors.Join(failure, errors.New("signal cohort exceeded declared actual provider call bound"))
		}
		proof["provider_calls"], proof["provider_usage_complete"] = calls, usageComplete
		proof["cases"], proof["machine_passed"] = cases, failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, err := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, err, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-signals.json"), raw))
	}()
	for _, task := range tasks {
		var hooks []func(string) error
		if task.Kind == "budget" {
			hooks = append(hooks, func(run string) error { return a.plan11ExhaustSignalBudget(ctx, env, run) })
		}
		row, err := a.plan11PreparedMetadataTask(ctx, env, task.Name, task.Prompt, nil, plan11SignalTraceAllowed, func(c context.Context, conversation string, sample *p5BenchmarkSample, p map[string]any) error {
			return a.plan11VerifySignalTask(c, env, task, d, conversation, sample, p)
		}, hooks...)
		cases = append(cases, row)
		if err != nil {
			failure = errors.Join(failure, err)
			if errors.Is(err, errPlan11NavigationBoundary) || row["actual_model_evaluated"] != true || ctx.Err() != nil {
				return failure
			}
		}
	}
	return failure
}

// Controlled test precondition only. It is never a product reset API or fake
// telemetry: actual subsequent catalogue/query calls must fail on this ledger.
func (a *App) plan11ExhaustSignalBudget(ctx context.Context, env *E2EEnvironment, run string) error {
	if !plan11SignalPhase(env.Options.Plan8Phase) || uuid.Validate(run) != nil {
		return errors.New("budget control requires its exact owned Run")
	}
	sql, err := plan11SignalBudgetSQL(run, env.State.Values["enterprise_id"], env.State.Values["admin_user_id"])
	if err != nil {
		return err
	}

	if _, err := a.postgresQuery(ctx, env, sql); err != nil {
		return err
	}
	raw, err := a.postgresQuery(ctx, env, `SELECT json_build_object('run_id',run_id,'scan_remaining',scan_remaining,'bytes_remaining',bytes_remaining,'rows_remaining',rows_remaining,'samples_remaining',samples_remaining,'calls_remaining',calls_remaining) FROM dashboard_run_budgets WHERE run_id='`+run+`'`)
	if err != nil || !strings.Contains(raw, run) {
		return errors.Join(err, errors.New("budget control was not persisted"))
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-signal-budget-initial.json"), []byte(raw))
}

func plan11SignalBudgetSQL(run, enterprise, owner string) (string, error) {
	for _, id := range []string{run, enterprise, owner} {
		if uuid.Validate(id) != nil {
			return "", errors.New("budget control requires canonical owned identities")
		}
	}
	return `DO $control$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM runs WHERE id='` + run + `' AND enterprise_id='` + enterprise + `' AND actor_user_id='` + owner + `') THEN RAISE EXCEPTION 'foreign test Run'; END IF;
 IF EXISTS(SELECT 1 FROM tool_calls WHERE run_id='` + run + `' AND tool_id='tool.invoke' AND (input->>'category' IN ('metric','log','trace') OR input->>'name' IN ('catalog','catalog.query','query'))) THEN RAISE EXCEPTION 'budget control started after a data call'; END IF;
 INSERT INTO dashboard_run_budgets(run_id,enterprise_id,owner_user_id,scan_remaining,bytes_remaining,rows_remaining,samples_remaining,calls_remaining) SELECT id,enterprise_id,actor_user_id,0,0,0,0,0 FROM runs WHERE id='` + run + `'
 ON CONFLICT(run_id) DO UPDATE SET scan_remaining=0,bytes_remaining=0,rows_remaining=0,samples_remaining=0,calls_remaining=0;
 END; $control$;`, nil
}
