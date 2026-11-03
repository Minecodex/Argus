package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Candidate goes first so a known legacy boundary failure cannot prevent
// evaluation of the implementation under review. Both profiles still stop
// the entire comparison immediately on a hard boundary failure.
func plan11ComparisonProfiles(selected ...string) []string {
	if len(selected) == 0 {
		return []string{"candidate", "legacy"}
	}
	return append([]string{}, selected...)
}

func plan11ComparisonModelNames(repeats int, selected ...string) []string {
	return plan11ComparisonSelectedModelNames(repeats, nil, selected)
}

func plan11ComparisonSelectedModelNames(repeats int, profiles, questions []string) []string {
	names := []string{}
	for _, profile := range plan11ComparisonProfiles(profiles...) {
		for _, question := range plan11ComparisonQuestions(questions) {
			for _, locale := range []string{"zh", "en"} {
				for repeat := 1; repeat <= repeats; repeat++ {
					names = append(names, fmt.Sprintf("plan11-compare-%s-%s-%s-%d", profile, question, locale, repeat))
				}
			}
		}
	}
	return names
}

type plan11ComparisonWindow struct {
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
	HighResolution bool      `json:"-"`
}

// Compare two business-discovery profiles serially on one owned installation.
// Writer pause freezes stored samples; Server/Worker always recover together.
func (a *App) plan11DeployedComparison(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	proof := map[string]any{"schema_version": "argus.plan11_profile_comparison/v1", "run_id": env.Options.RunID, "passed": false, "plan_complete": false, "release_eligible": false, "repeats": env.Options.Plan11CompareRepeats, "profile_order": plan11ComparisonProfiles(env.Options.Plan11CompareProfiles...), "cases": []map[string]any{}}
	proof["delivery_contract"] = plan11DeliveryContract(env.Options.Plan11DeliverySummary)
	proof["comparative_output_contract_unchanged"] = !env.Options.Plan11DeliverySummary
	defer func() {
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, err := json.MarshalIndent(proof, "", "  ")
		if err == nil {
			err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-profile-comparison.json"), raw)
		}
		failure = errors.Join(failure, err)
	}()
	if err := plan11ValidateDeliverySummary(env.Options.Plan11DeliverySummary, env.Options.Suite, env.Options.Plan8Phase, env.Options.Plan11CompareProfiles); err != nil {
		return err
	}
	if err := plan11ValidateHighResolution(env.Options.Plan11HighResolution, env.Options.Plan11DeliverySummary, env.Options.Suite, env.Options.Plan8Phase, env.Options.Plan11CompareProfiles); err != nil {
		return err
	}
	if !plan11ComparisonEnvironmentAllowed(env, h) {
		return errors.New("comparison requires its leased owned evaluation environment")
	}
	if env.Options.Plan11CompareRepeats > 0 {
		if _, err := a.plan7Model(ctx, env); err != nil {
			return fmt.Errorf("comparison model prerequisite: %w", err)
		}
	}
	active, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM runs WHERE status IN ('pending','running','waiting_input','waiting_approval','waiting_system')")
	if err != nil || strings.TrimSpace(active) != "0" {
		return errors.New("comparison cannot switch an installation with active Runs")
	}
	if err = a.plan11WaitModelAccounting(ctx, env); err != nil {
		return err
	}
	legacy, err := a.loadPlan11LegacyImage(ctx, env)
	if err != nil {
		return err
	}
	states := []*plan11DeploymentState{}
	for _, name := range []string{"argus-server", "argus-worker"} {
		state, err := plan11CaptureDeployment(ctx, env, env.SystemNS, name)
		if err != nil {
			return err
		}
		states = append(states, state)
	}
	writer, err := plan11CaptureDeployment(ctx, env, env.ObservNS, "argus-telemetry-writer")
	if err != nil {
		return err
	}
	// Register recovery before the first mutation, including partial switches.
	defer func() {
		recovery, cancel := plan11RecoveryContext()
		defer cancel()
		backendErr := plan11SwitchBackend(recovery, env, states, "", true)
		writerErr := plan11SetReplicas(recovery, env, writer, *writer.Original.Spec.Replicas)
		proof["candidate_restored"] = backendErr == nil
		proof["writer_restored"] = writerErr == nil
		failure = errors.Join(failure, plan11ProfileRecoveryError(errors.Join(backendErr, writerErr)))
	}()
	if env.Options.Plan11HighResolution {
		fmt.Fprintln(a.stdout, "High-resolution evidence: allow one actual Collector interval minute before freezing; no synthetic samples")
		if err := waitContext(ctx, time.Minute); err != nil {
			return err
		}
	}
	if err = plan11SetReplicas(ctx, env, writer, 0); err != nil {
		return err
	}
	window := plan11ComparisonWindow{To: time.Now().UTC().Add(-5 * time.Second).Truncate(time.Second), HighResolution: env.Options.Plan11HighResolution}
	window.From = window.To.Add(-5 * time.Minute)
	proof["window"], proof["host_id"], proof["source_id"] = window, h.HostID, h.SourceID
	proof["native_query_step_seconds"], proof["large_cpu_original_required"] = plan11NativeStep(window), window.HighResolution
	proof["model"] = env.Options.RealModel.ModelID
	proof["model_parameters"] = map[string]any{"protocol": env.Options.RealModel.Protocol, "reasoning_effort": env.Options.RealModel.ReasoningEffort, "max_output_tokens": env.Options.RealModel.MaxOutput, "endpoint_ip": env.Options.RealModel.EndpointIP, "disable_mlkem": env.Options.RealModel.DisableMLKEM}
	units, err := a.plan11ComparisonUnits(ctx, env, h, window)
	if err != nil {
		return err
	}
	encodedUnits, _ := json.Marshal(units)
	env.State.Values["plan11_comparison_units"] = string(encodedUnits)
	proof["reference_catalog_units"] = units
	reference, err := a.plan11ReferenceFingerprint(ctx, env, h, window)
	if err != nil {
		return err
	}
	proof["reference_before"] = reference
	proof["questions"] = plan11ComparisonQuestions(env.Options.Plan11CompareQuestions)
	for _, profile := range plan11ComparisonProfiles(env.Options.Plan11CompareProfiles...) {
		if err = a.plan11WaitModelAccounting(ctx, env); err != nil {
			return err
		}
		image, restoring := legacy, false
		if profile == "candidate" {
			image, restoring = "", true
		}
		if err = plan11SwitchBackend(ctx, env, states, image, restoring); err != nil {
			return err
		}
		proof[profile+"_deployments"] = plan11ProfileDescription(states)
		before, err := a.plan11ReferenceFingerprint(ctx, env, h, window)
		if err != nil || before != reference {
			return errors.Join(err, errors.New("comparison data changed before profile tasks"))
		}
		for _, question := range plan11ComparisonQuestions(env.Options.Plan11CompareQuestions) {
			for _, locale := range []string{"zh", "en"} {
				for repeat := 1; repeat <= env.Options.Plan11CompareRepeats; repeat++ {
					name := fmt.Sprintf("plan11-compare-%s-%s-%s-%d", profile, question, locale, repeat)
					result, caseErr := a.plan11ComparisonCase(ctx, env, h, window, profile, question, locale, name)
					proof["cases"] = append(proof["cases"].([]map[string]any), result)
					if caseErr != nil {
						fmt.Fprintf(a.stdout, "Comparison %s recorded failure: %s\n", name, string(redactDiagnostic([]byte(caseErr.Error()))))
					}
					if ctx.Err() != nil {
						return errors.Join(caseErr, context.Cause(ctx))
					}
					if result["hard_failure"] == true {
						return errors.Join(caseErr, errors.New("comparison hard boundary failed"))
					}
				}
			}
		}
		after, err := a.plan11ReferenceFingerprint(ctx, env, h, window)
		if err != nil || after != reference {
			return errors.Join(err, errors.New("comparison data changed during profile tasks"))
		}
		proof[profile+"_reference_after"] = after
	}
	metrics, err := plan11ComparisonMetricsForProfiles(proof["cases"].([]map[string]any), env.Options.Plan11CompareRepeats, env.Options.Plan11CompareProfiles, env.Options.Plan11CompareQuestions)
	proof["metrics"] = metrics
	proof["same_resources_and_reference_data"] = true
	proof["scope"] = "declared bilingual read-only questions, same frozen stored samples; shared Query Engine/Collector/Workspace; candidate then legacy Server/Worker"
	return err
}

// A cancelled/terminal Run can still have an in-flight soft compaction or quota
// settlement. Do not kill that accounting cohort when switching profiles.
func (a *App) plan11WaitModelAccounting(ctx context.Context, env *E2EEnvironment) error {
	query := `SELECT
 (SELECT count(*) FROM model_calls WHERE status IN ('reserved','running'))+
 (SELECT count(*) FROM runtime_tasks WHERE queue='compaction' AND status IN ('pending','leased','running'))+
 (SELECT count(*) FROM model_quota_reservations q JOIN model_calls c ON c.id=q.model_call_id WHERE c.status='succeeded' AND q.status='active');`
	if err := a.waitPostgresValue(ctx, env, query, "0", 3*time.Minute); err != nil {
		return fmt.Errorf("comparison background accounting did not settle: %w", err)
	}
	return nil
}

func (a *App) plan11ReferenceFingerprint(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, window plan11ComparisonWindow) (string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	results := []any{}
	for _, query := range []string{`system_memory_utilization{state="used"}`, `system_cpu_utilization{state="idle"}`, `system_network_io`} {
		value, err := client.JSON(ctx, "comparison-reference", "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": query, "resource_ids": []string{h.HostID}, "time_range": window, "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return "", err
		}
		data, ok := value["data"].(map[string]any)
		if !ok || value["status"] != "success" {
			return "", errors.New("comparison reference query failed")
		}
		rows, ok := data["result"].([]any)
		if !ok || len(rows) == 0 {
			return "", errors.New("comparison reference query has no actual samples")
		}
		sort.Slice(rows, func(i, j int) bool {
			left, _ := json.Marshal(rows[i])
			right, _ := json.Marshal(rows[j])
			return string(left) < string(right)
		})
		data["result"] = rows
		results = append(results, map[string]any{"query": query, "data": data})
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func (a *App) plan11ComparisonUnits(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, window plan11ComparisonWindow) (map[string]string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	units := map[string]string{}
	cursor := ""
	for page := 0; page < 10; page++ {
		value, err := client.JSON(ctx, "comparison-catalog-reference", "enterprise", http.MethodPost, "/enterprise/telemetry/query/catalog", 200, map[string]any{"scope": map[string]any{"mode": "explicit", "resources": []map[string]string{{"type": "host", "id": h.HostID}}}, "signal": "metrics", "kind": "metrics", "observed_only": true, "from": window.From, "to": window.To, "limit": 100, "cursor": cursor}, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		rows, _ := value["metrics"].([]any)
		for _, row := range rows {
			metric, _ := row.(map[string]any)
			name, _ := metric["name"].(string)
			unit, _ := metric["unit"].(string)
			if previous, ok := units[name]; ok && previous != unit {
				return nil, errors.New("comparison source metric has ambiguous units")
			}
			units[name] = unit
		}
		cursor, _ = value["next_cursor"].(string)
		if cursor == "" {
			if complete, _ := value["complete"].(bool); !complete || len(units) == 0 {
				return nil, errors.New("comparison reference catalog is incomplete")
			}
			return units, nil
		}
	}
	return nil, errors.New("comparison reference catalog exceeded its explicit page budget")
}
