package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Pauses are confined to m4e2e builds and an explicitly enabled temporary
// Worker. Real engines, storage, leases and Workspace RPC still do all work.
func (a *App) preparePlanV2Faults(ctx context.Context, env *E2EEnvironment) error {
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("query fault namespace ownership is not established")
	}
	_, err = a.postgresQuery(ctx, env, `CREATE TABLE argus_e2e_dashboard_checkpoints(enterprise_id uuid NOT NULL,conversation_id uuid NOT NULL,stage text NOT NULL,job_id uuid,entered_at timestamptz,hits int NOT NULL DEFAULT 0,released boolean NOT NULL DEFAULT false,PRIMARY KEY(enterprise_id,conversation_id,stage)); GRANT SELECT,UPDATE ON argus_e2e_dashboard_checkpoints TO PUBLIC;`)
	if err != nil {
		return err
	}
	container := map[string]any{"name": "argus-worker", "env": []any{map[string]any{"name": "ARGUS_E2E_QUERY_CHECKPOINTS", "value": "1"}}}
	patch := map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{container}}}}}
	if err := env.Kube.PatchDeployment(ctx, env.SystemNS, "argus-worker", patch); err != nil {
		return err
	}
	return env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 3*time.Minute)
}

type planV2FaultCase struct {
	Name         string `json:"name"`
	Passed       bool   `json:"passed"`
	Job, Attempt string
	Facts        map[string]any `json:"facts"`
}

func (a *App) runPlanV2Faults(ctx context.Context, env *E2EEnvironment) (failure error) {
	spec, err := planV2ThreeSignals()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 interrupted query files", spec)
	if err != nil {
		return err
	}
	cases := []planV2FaultCase{}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"kind": "deployed-worker-pvc-faults/v1", "cases": cases, "planned": 6, "passed": failure == nil && len(cases) == 6, "actual_model_evaluated": false}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "planv2-inflight-faults.json"), raw))
	}()
	for _, stage := range []string{"fetched_target", "delivered_file"} {
		proof, err := a.planV2WorkerCrash(ctx, env, id, stage)
		proof.Passed = err == nil
		if err != nil {
			failure = errors.Join(failure, fmt.Errorf("%s crash: %w", stage, err))
		}
		cases = append(cases, proof)
	}
	for _, kind := range []string{"cancel", "corrupt", "quota", "archive"} {
		proof, err := a.planV2DeliveryFault(ctx, env, id, kind)
		proof.Passed = err == nil
		if err != nil {
			failure = errors.Join(failure, fmt.Errorf("%s fault: %w", kind, err))
		}
		cases = append(cases, proof)
	}
	return failure
}

func (a *App) planV2Gate(ctx context.Context, env *E2EEnvironment, conversation, stage string, released bool) error {
	if uuid.Validate(conversation) != nil {
		return fmt.Errorf("invalid checkpoint conversation")
	}
	_, err := a.postgresQuery(ctx, env, fmt.Sprintf(`INSERT INTO argus_e2e_dashboard_checkpoints(enterprise_id,conversation_id,stage,released) VALUES('%s','%s','%s',%t) ON CONFLICT(enterprise_id,conversation_id,stage) DO UPDATE SET released=excluded.released`, env.State.Values["enterprise_id"], conversation, stage, released))
	return err
}

func (a *App) releasePlanV2Gate(ctx context.Context, env *E2EEnvironment, conversation, stage string) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return a.planV2Gate(cleanup, env, conversation, stage, true)
}

func (a *App) planV2StartFault(ctx context.Context, env *E2EEnvironment, dashboardID, conversation string) (dashboard.QueryJobView, string, error) {
	client, _ := scenarioHTTP(env)
	path := "/conversations/" + conversation + "/dashboard-queries"
	value, err := client.JSON(ctx, "p2-fault-start", "enterprise", http.MethodPost, path, 202, map[string]any{"dashboard_id": dashboardID, "parameters": map[string]any{"resource_ids": []string{env.State.Values["m3_cluster_id"]}}}, enterpriseHeaders(env, "p2-fault-"+uuid.NewString()))
	if err != nil {
		return dashboard.QueryJobView{}, path, err
	}
	raw, _ := json.Marshal(value)
	var job dashboard.QueryJobView
	err = json.Unmarshal(raw, &job)
	return job, path + "/" + job.ID.String(), err
}

func (a *App) waitPlanV2Gate(ctx context.Context, env *E2EEnvironment, conversation, stage string) error {
	return a.waitPostgresValue(ctx, env, fmt.Sprintf(`SELECT count(*) FROM argus_e2e_dashboard_checkpoints WHERE conversation_id='%s' AND stage='%s' AND entered_at IS NOT NULL`, conversation, stage), "1", 3*time.Minute)
}

func (a *App) planV2Job(ctx context.Context, env *E2EEnvironment, path string) (dashboard.QueryJobView, error) {
	client, _ := scenarioHTTP(env)
	value, err := client.JSON(ctx, "p2-fault-status", "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return dashboard.QueryJobView{}, err
	}
	raw, _ := json.Marshal(value)
	var job dashboard.QueryJobView
	err = json.Unmarshal(raw, &job)
	return job, err
}

func (a *App) verifyPlanV2FileBytes(ctx context.Context, env *E2EEnvironment, conversation string, job dashboard.QueryJobView) error {
	if job.Manifest == nil || !job.Manifest.Complete || job.Status != "complete" || len(job.Files) != 4 {
		return fmt.Errorf("incomplete recovered file job")
	}
	client, _ := scenarioHTTP(env)
	for _, file := range job.Files {
		if file.WorkspaceFileID == nil || !strings.Contains(file.Path, job.Manifest.AttemptID.String()) {
			return fmt.Errorf("recovered file lost its attempt")
		}
		data, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+file.WorkspaceFileID.String()+"/content", nil, 200, "")
		if err != nil {
			return err
		}
		if !json.Valid(data) || int64(len(data)) != file.Bytes || planV2Hash(data) != file.Hash {
			return fmt.Errorf("recovered bytes differ from manifest")
		}
	}
	return nil
}

func (a *App) planV2WorkerCrash(ctx context.Context, env *E2EEnvironment, id, stage string) (proof planV2FaultCase, failure error) {
	proof = planV2FaultCase{Name: stage + "_worker_crash", Facts: map[string]any{}}
	convo, err := a.p5Conversation(ctx, env, proof.Name)
	if err != nil {
		return proof, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		if err := a.p5DeleteWorkspace(cleanup, env, convo); failure == nil {
			failure = err
		}
	}()
	if err = a.planV2Gate(ctx, env, convo, stage, false); err != nil {
		return proof, err
	}
	defer func() { failure = errors.Join(failure, a.releasePlanV2Gate(ctx, env, convo, stage)) }()
	job, path, err := a.planV2StartFault(ctx, env, id, convo)
	if err != nil {
		return proof, err
	}
	if err = a.waitPlanV2Gate(ctx, env, convo, stage); err != nil {
		return proof, err
	}
	oldAttempt, err := a.postgresQuery(ctx, env, "SELECT attempt_id::text FROM dashboard_query_jobs WHERE id='"+job.ID.String()+"';")
	if err != nil {
		return proof, err
	}
	oldAttempt = strings.TrimSpace(oldAttempt)
	before, err := a.planV2Job(ctx, env, path)
	if err != nil {
		return proof, err
	}
	if stage == "delivered_file" {
		delivered := 0
		for _, file := range before.Files {
			if file.WorkspaceFileID != nil {
				delivered++
			}
		}
		if before.Manifest == nil || delivered < 1 || delivered >= len(before.Files) {
			return proof, fmt.Errorf("crash did not interrupt partial delivery")
		}
		proof.Facts["delivered_before_crash"] = delivered
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-worker"})
	if err != nil || len(pods.Items) != 1 {
		return proof, fmt.Errorf("expected one isolated Worker before crash: %v", err)
	}
	pod := pods.Items[0]
	if err := env.Kube.VerifyDeploymentPod(ctx, env.SystemNS, "argus-worker", env.ReleaseID, pod); err != nil {
		return proof, err
	}
	zero := int64(0)
	uid := pod.UID
	if err = env.Kube.Client.CoreV1().Pods(env.SystemNS).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		return proof, err
	}
	if err = a.planV2Gate(ctx, env, convo, stage, true); err != nil {
		return proof, err
	}
	if err = env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 3*time.Minute); err != nil {
		return proof, err
	}
	after, err := a.waitPlanV2Files(ctx, env, path)
	if err != nil {
		return proof, err
	}
	if err = a.verifyPlanV2FileBytes(ctx, env, convo, after); err != nil {
		return proof, err
	}
	if stage == "fetched_target" {
		if after.Manifest.AttemptID.String() == oldAttempt {
			return proof, fmt.Errorf("unsealed acquisition reused its abandoned attempt")
		}
		if err = a.waitPostgresValue(ctx, env, "SELECT status FROM dashboard_query_attempts WHERE id='"+oldAttempt+"';", "abandoned", time.Minute); err != nil {
			return proof, err
		}
	} else if after.Manifest.AttemptID.String() != oldAttempt {
		return proof, fmt.Errorf("sealed delivery refetched data")
	}
	proof.Job, proof.Attempt = job.ID.String(), after.Manifest.AttemptID.String()
	proof.Facts["old_attempt"], proof.Facts["hashes_verified"] = oldAttempt, true
	return proof, nil
}
