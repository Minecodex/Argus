package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This is a deterministic transport/tool acceptance, not an LLM quality test.
// Acquisition uses the public published-query API; delivery uses the deployed
// Worker, object store, Workspace RPC and actual PVC. No in-memory substitute.
func (a *App) runPlanV2Files(ctx context.Context, env *E2EEnvironment) error {
	if err := a.configureP5Workspace(ctx, env); err != nil {
		return err
	}
	spec, err := planV2ThreeSignals()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 three signal files", spec)
	if err != nil {
		return err
	}
	env.State.Values["p2_files_dashboard_id"] = id
	convo, err := a.p5Conversation(ctx, env, "PlanV2 query files")
	if err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	path := "/conversations/" + convo + "/dashboard-queries"
	input := map[string]any{"dashboard_id": id, "parameters": map[string]any{"resource_ids": []string{env.State.Values["m3_cluster_id"]}}}
	queued, err := client.JSON(ctx, "p2-files-start", "enterprise", http.MethodPost, path, 202, input, enterpriseHeaders(env, "p2-files-start"))
	if err != nil {
		return err
	}
	jobID, err := stringField(queued, "id")
	if err != nil {
		return err
	}
	job, err := a.waitPlanV2Files(ctx, env, path+"/"+jobID)
	if err != nil {
		return err
	}
	observed, _ := json.MarshalIndent(job, "", "  ")
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "planv2-files-observed.json"), observed); err != nil {
		return err
	}
	if job.Manifest == nil || !job.Manifest.Complete || job.Manifest.AnalysisStatus != "not_analyzed" || len(job.Manifest.Execution.Panels) != 3 || len(job.Files) != 4 {
		return fmt.Errorf("PlanV2 file coverage/analysis state invalid")
	}
	for _, p := range job.Manifest.Execution.Panels {
		if p.Status != "success" || len(p.Sources) == 0 {
			return fmt.Errorf("PlanV2 file panel %s did not execute successfully: %s", p.ID, p.Status)
		}
	}
	manifestPath, samplePath := "", ""
	for _, f := range job.Files {
		if f.WorkspaceFileID == nil || f.WorkspaceID == nil || f.SourceRef != dashboard.QuerySourcePrefix+job.ID.String()+"/"+job.Manifest.AttemptID.String() {
			return fmt.Errorf("PlanV2 delivery did not retain immutable provenance")
		}
		body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+f.WorkspaceFileID.String()+"/content", nil, 200, "")
		if err != nil {
			return err
		}
		if int64(len(body)) != f.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != f.Hash || !json.Valid(body) {
			return fmt.Errorf("PlanV2 downloaded bytes differ from frozen query: %s", f.PanelID)
		}
		if f.Kind == "manifest" {
			manifestPath = strings.TrimPrefix(f.Path, "/workspace/")
		}
		if f.Kind == "data" && f.PanelID == "logs" {
			samplePath = strings.TrimPrefix(f.Path, "/workspace/")
		}
	}
	if manifestPath == "" || samplePath == "" {
		return fmt.Errorf("PlanV2 manifest was not delivered")
	}
	// No filesystem inspection through kubectl: the ordinary offline tools read
	// the files and publish their computed proof through normal source checks.
	files, _ := json.Marshal(job.Files)
	command := strings.ReplaceAll(planV2FileProof, "FILES_BASE64", base64.StdEncoding.EncodeToString(files))
	run, err := a.p5Run(ctx, env, convo, "p2-read-query-files", []p5ToolStep{
		// Read a bounded real sample. The script reads and validates the entire
		// manifest/data on disk instead of duplicating it in Replay's context.
		{"read", map[string]any{"path": samplePath, "start_line": 1, "end_line": 1}},
		{"bash", map[string]any{"command": command}},
		{"tool.invoke", map[string]any{"category": "workflow", "name": "publish_file", "arguments": map[string]any{"path": "planv2-proof.json", "name": "planv2-proof.json", "media_type": "application/json"}}},
	}, []string{})
	if err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id WHERE t.run_id='"+run+"' AND t.tool_id='bash' AND t.status='succeeded' AND r.projection->'summary'->>'exit_code'='0';", "1", time.Minute); err != nil {
		return fmt.Errorf("PlanV2 offline checksum/content proof failed: %w", err)
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND tool_id='read' AND status='succeeded';", "1", time.Minute); err != nil {
		return err
	}
	pod, err := a.p5WorkspacePod(ctx, env, convo)
	if err != nil {
		return err
	}
	pvcIdentity := ""
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim == nil {
			continue
		}
		claim, err := env.Kube.Client.CoreV1().PersistentVolumeClaims(pod.Namespace).Get(ctx, v.PersistentVolumeClaim.ClaimName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if claim.Status.Phase != "Bound" {
			return fmt.Errorf("PlanV2 query workspace PVC is not Bound")
		}
		pvcIdentity = pod.Namespace + "/" + claim.Name + "/" + string(claim.UID)
	}
	if pvcIdentity == "" {
		return fmt.Errorf("PlanV2 offline tools did not mount a real PVC")
	}
	delivery, err := a.postgresQuery(ctx, env, "SELECT id::text FROM file_deliveries WHERE run_id='"+run+"' AND name='planv2-proof.json' AND cardinality(source_result_refs)=1;")
	if err != nil || strings.TrimSpace(delivery) == "" {
		return fmt.Errorf("PlanV2 proof was not published with its source: %v", err)
	}
	proof, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/deliveries/"+strings.TrimSpace(delivery)+"/content", nil, 200, "")
	if err != nil {
		return err
	}
	var checked struct {
		JobID      string           `json:"job_id"`
		RevisionID string           `json:"revision_id"`
		Files      []map[string]any `json:"files"`
	}
	if json.Unmarshal(proof, &checked) != nil || checked.JobID != job.ID.String() || checked.RevisionID != job.RevisionID.String() || len(checked.Files) != 3 {
		return fmt.Errorf("PlanV2 published proof omitted query data")
	}
	if err := a.verifyPlanV2FileRecovery(ctx, env, convo, input, job); err != nil {
		return err
	}
	if err := a.verifyPlanV2FileAudit(ctx, env, job); err != nil {
		return err
	}
	// A different owned conversation must not acquire the first one's files.
	other, err := a.p5Conversation(ctx, env, "PlanV2 isolated files")
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p2-files-cross-conversation", "enterprise", http.MethodGet, "/conversations/"+other+"/dashboard-queries/"+jobID, 403, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "planv2-file-proof.json"), proof); err != nil {
		return err
	}
	encoded, _ := json.MarshalIndent(map[string]any{"kind": "deterministic-real-workspace/v1", "job": job, "offline_run": run, "workspace_pvc": pvcIdentity, "download_hashes_verified": true, "worker_redis_recovery": true, "queued_cancel": true, "cross_conversation_denied": true, "actual_model_evaluated": false}, "", "  ")
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "planv2-files.json"), encoded); err != nil {
		return err
	}
	return a.p5DeleteWorkspace(ctx, env, convo)
}

func (a *App) waitPlanV2Files(ctx context.Context, env *E2EEnvironment, path string) (dashboard.QueryJobView, error) {
	client, _ := scenarioHTTP(env)
	deadline := time.Now().Add(5 * time.Minute)
	for {
		result, err := client.JSON(ctx, "p2-files-progress", "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return dashboard.QueryJobView{}, err
		}
		raw, _ := json.Marshal(result)
		var view dashboard.QueryJobView
		if err := json.Unmarshal(raw, &view); err != nil {
			return view, err
		}
		if view.Status == "complete" {
			return view, nil
		}
		if view.Status == "failed" || view.Status == "partial" || view.Status == "cancelled" || time.Now().After(deadline) {
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "planv2-files-failure.json"), raw)
			return view, fmt.Errorf("PlanV2 query delivery ended %s (%s)", view.Status, view.ErrorCode)
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return view, err
		}
	}
}

func (a *App) verifyPlanV2FileRecovery(ctx context.Context, env *E2EEnvironment, convo string, input map[string]any, completed dashboard.QueryJobView) (returnErr error) {
	client, _ := scenarioHTTP(env)
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil {
		return err
	}
	// The caller's release owns this namespace; never pause an external worker.
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("PlanV2 worker namespace ownership is not established")
	}
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		returnErr = errors.Join(returnErr, env.Kube.ScaleDeployment(cleanup, env.SystemNS, "argus-worker", replicas))
	}()
	if err := env.Kube.ScaleDeployment(ctx, env.SystemNS, "argus-worker", 0); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Minute)
	for {
		pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-worker"})
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("PlanV2 Worker did not stop before queued cancellation")
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
	path := "/conversations/" + convo + "/dashboard-queries"
	queued, err := client.JSON(ctx, "p2-cancel-queued-start", "enterprise", http.MethodPost, path, 202, input, enterpriseHeaders(env, "p2-cancel-queued-start"))
	if err != nil {
		return err
	}
	id, _ := stringField(queued, "id")
	cancelled, err := client.JSON(ctx, "p2-cancel-queued", "enterprise", http.MethodPost, path+"/"+id+"/cancel", 200, nil, enterpriseHeaders(env, ""))
	if err != nil || cancelled["status"] != "cancelled" {
		return fmt.Errorf("PlanV2 queued query cancellation failed: %v", err)
	}
	password, err := dataCredentialValue(ctx, env, "redis-password")
	if err != nil {
		return err
	}
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-redis", "redis", "redis-cli", "-a", password, "FLUSHALL"); err != nil {
		return err
	}
	if err := env.Kube.ScaleDeployment(ctx, env.SystemNS, "argus-worker", replicas); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 3*time.Minute); err != nil {
		return err
	}
	after, err := a.waitPlanV2Files(ctx, env, path+"/"+completed.ID.String())
	if err != nil || after.Manifest == nil || after.Manifest.AttemptID != completed.Manifest.AttemptID || len(after.Files) != len(completed.Files) {
		return fmt.Errorf("PlanV2 completed query changed after Worker/Redis recovery: %v", err)
	}
	for _, f := range after.Files {
		originalHash := ""
		for _, original := range completed.Files {
			if original.ID == f.ID {
				originalHash = original.Hash
			}
		}
		if f.WorkspaceFileID == nil || originalHash == "" || originalHash != f.Hash {
			return fmt.Errorf("PlanV2 recovered file metadata changed")
		}
		body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+f.WorkspaceFileID.String()+"/content", nil, 200, "")
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != originalHash {
			return fmt.Errorf("PlanV2 recovered query file changed: %v", err)
		}
	}
	return a.waitPostgresValue(ctx, env, "SELECT status||':'||(SELECT count(*) FROM dashboard_query_attempts WHERE job_id=j.id)::text FROM dashboard_query_jobs j WHERE id='"+id+"';", "cancelled:0", time.Minute)
}

func planV2ThreeSignals() (dashboard.Spec, error) {
	spec := dashboard.EmptySpec()
	for i, item := range []struct {
		signal, kind, query string
		language            queryengine.Language
	}{
		{"metrics", "timeseries", "argus_m7_e2e_gauge_planv2", queryengine.LanguagePromQL},
		{"logs", "logs", "* | limit 100", queryengine.LanguageKQL},
		{"traces", "trace_list", `query { queryTraces(pageSize:100) { total traces { traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount } } }`, queryengine.LanguageTrace},
	} {
		mode := ""
		if item.signal == "metrics" {
			mode = "range"
		}
		spec.Panels = append(spec.Panels, dashboard.Panel{ID: item.signal, Title: item.signal, Signal: item.signal, Type: item.kind, AuthoringMode: "dsl", ApplicableResourceTypes: []string{"kubernetes_cluster"}, SourceBinding: dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, LocalFilters: []dashboard.LocalFilter{}, DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Layout: dashboard.Rectangle{X: 0, Y: i * 52, W: 12, H: 48, MinW: 3, MinH: 8}, Targets: []dashboard.Target{{ID: "main", Language: item.language, QueryMode: mode, SourceDefinition: dashboard.Definition{DSL: &dashboard.DSL{Expression: item.query}}, RangeStepPolicy: dashboard.StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}, ParameterBindings: []dashboard.ParameterBinding{}}}})
	}
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("three-signal fixture invalid: %+v", report.Issues)
	}
	return spec, nil
}

func (a *App) publishPlanV2Fixture(ctx context.Context, env *E2EEnvironment, name string, spec dashboard.Spec) (string, error) {
	client, _ := scenarioHTTP(env)
	key := p5RequestKey("p2-fixture", name)
	input := planV2DraftRequest(spec)
	input["name"] = name
	input["description"] = "Published three-signal queries for durable file acceptance."
	draft, err := client.JSON(ctx, key+"-draft", "enterprise", http.MethodPost, planV2DraftPath, 201, input, enterpriseHeaders(env, ""))
	if err != nil {
		return "", err
	}
	id, err := stringField(draft, "id")
	if err != nil {
		return "", err
	}
	preview, err := client.JSON(ctx, key+"-preview", "enterprise", http.MethodPost, planV2DraftPath+"/"+id+"/preview", 201, map[string]any{"expected_version": draft["draft_version"]}, enterpriseHeaders(env, key+"-preview"))
	if err != nil {
		return "", err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return "", err
	}
	if _, err = a.confirmPendingAction(ctx, env, key+"-publish", ref); err != nil {
		return "", err
	}
	stored, err := client.JSON(ctx, key+"-published-draft", "enterprise", http.MethodGet, planV2DraftPath+"/"+id, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return "", err
	}
	return stringField(stored, "dashboard_id")
}

const planV2FileProof = `python - <<'PY'
import base64, hashlib, json
from pathlib import Path
files = json.loads(base64.b64decode('FILES_BASE64'))
root = Path('/workspace').resolve()
proof = {'files': []}
def count(value):
    if isinstance(value, list): return len(value)
    if isinstance(value, dict):
        for key in ('rows', 'traces', 'result'):
            if isinstance(value.get(key), list): return len(value[key])
        return max([count(v) for v in value.values()] or [0])
    return 0
for item in files:
    path = Path(item['path']).resolve()
    assert root in path.parents
    raw = path.read_bytes()
    assert len(raw) == item['bytes'] and hashlib.sha256(raw).hexdigest() == item['sha256']
    value = json.loads(raw)
    if item['kind'] == 'manifest':
        assert value['complete'] and value['analysis_status'] == 'not_analyzed'
        proof.update(job_id=value['job_id'], revision_id=value['execution']['revision_id'], attempt_id=value['attempt_id'])
    else:
        n = count(value)
        assert n > 0, item['panel_id']
        proof['files'].append(dict(panel_id=item['panel_id'], records=n, sha256=item['sha256'], bytes=len(raw)))
assert {f['panel_id'] for f in proof['files']} == {'metrics','logs','traces'}
Path('/workspace/planv2-proof.json').write_text(json.dumps(proof))
print('planv2-query-files-verified')
PY`
