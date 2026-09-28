package argusdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"net/http"
	"strings"
	"time"
)

func planV2Hash(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func (a *App) planV2DeliveryFault(ctx context.Context, env *E2EEnvironment, id, kind string) (proof planV2FaultCase, failure error) {
	proof = planV2FaultCase{Name: kind, Facts: map[string]any{}}
	convo, err := a.p5Conversation(ctx, env, "PlanV2 "+kind)
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
	stage := "materialized"
	if kind == "cancel" {
		stage = "fetched_target"
	}
	if kind == "archive" {
		stage = "delivered_file"
	}
	if err := a.planV2Gate(ctx, env, convo, stage, false); err != nil {
		return proof, err
	}
	defer func() { failure = errors.Join(failure, a.releasePlanV2Gate(ctx, env, convo, stage)) }()
	job, path, err := a.planV2StartFault(ctx, env, id, convo)
	if err != nil {
		return proof, err
	}
	proof.Job = job.ID.String()
	if err := a.waitPlanV2Gate(ctx, env, convo, stage); err != nil {
		return proof, err
	}
	before, err := a.planV2Job(ctx, env, path)
	if err != nil {
		return proof, err
	}
	client, _ := scenarioHTTP(env)
	if kind == "cancel" {
		started := time.Now()
		if _, err := client.JSON(ctx, "p2-cancel-inflight", "enterprise", http.MethodPost, path+"/cancel", 200, nil, enterpriseHeaders(env, "")); err != nil {
			return proof, err
		}
		if err := a.waitPostgresValue(ctx, env, "SELECT j.status||':'||t.status FROM dashboard_query_jobs j JOIN runtime_tasks t ON t.id=j.task_id WHERE j.id='"+job.ID.String()+"';", "cancelled:succeeded", 10*time.Second); err != nil {
			return proof, err
		}
		proof.Facts["cancel_milliseconds"] = time.Since(started).Milliseconds()
		if time.Since(started) > 5*time.Second {
			return proof, fmt.Errorf("in-flight cancellation exceeded five seconds")
		}
		if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM dashboard_query_files WHERE job_id='"+job.ID.String()+"';", "0", time.Second); err != nil {
			return proof, err
		}
		return proof, nil
	}
	if before.Manifest == nil {
		return proof, fmt.Errorf("fault must happen after sealing")
	}
	proof.Attempt = before.Manifest.AttemptID.String()
	if kind == "archive" {
		if err := a.planV2DashboardLifecycle(ctx, env, id, "archive"); err != nil {
			return proof, err
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			if err := a.planV2DashboardLifecycle(cleanup, env, id, "restore"); failure == nil {
				failure = err
			}
		}()
		if err := a.planV2Gate(ctx, env, convo, stage, true); err != nil {
			return proof, err
		}
		if err := a.waitPostgresValue(ctx, env, "SELECT status FROM dashboard_query_jobs WHERE id='"+job.ID.String()+"';", "failed", time.Minute); err != nil {
			return proof, err
		}
		if _, err := client.JSON(ctx, "p2-archived-job-denied", "enterprise", http.MethodGet, path, 409, nil, enterpriseHeaders(env, "")); err != nil {
			return proof, err
		}
		for _, file := range before.Files {
			if file.WorkspaceFileID != nil {
				if _, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+file.WorkspaceFileID.String()+"/content", nil, 403, ""); err != nil {
					return proof, err
				}
				proof.Facts["delivered_file_denied"] = true
				break
			}
		}
		if proof.Facts["delivered_file_denied"] != true {
			return proof, fmt.Errorf("archive did not interrupt a delivered file")
		}
		return proof, nil
	}
	var repair func() error
	if kind == "corrupt" {
		repair, err = a.corruptPlanV2Chunk(ctx, env, convo, job.ID.String(), proof.Attempt)
		if err != nil {
			return proof, err
		}
		defer func() {
			if repair != nil {
				_ = repair()
			}
		}()
	} else if kind == "quota" {
		if err := a.p5BashProof(ctx, env, convo, "p2-fill-quota-"+convo, planV2FillQuota); err != nil {
			return proof, err
		}
		repair = func() error { return a.p5BashProof(ctx, env, convo, "p2-free-quota-"+convo, planV2FreeQuota) }
	} else {
		return proof, fmt.Errorf("unknown delivery fault")
	}
	if err := a.planV2Gate(ctx, env, convo, stage, true); err != nil {
		return proof, err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT status FROM dashboard_query_jobs WHERE id='"+job.ID.String()+"';", "failed", 3*time.Minute); err != nil {
		return proof, err
	}
	failed, err := a.planV2Job(ctx, env, path)
	if err != nil {
		return proof, err
	}
	proof.Facts["error_code"] = failed.ErrorCode
	if failed.Manifest == nil || failed.Manifest.AttemptID != before.Manifest.AttemptID {
		return proof, fmt.Errorf("failure replaced the sealed attempt")
	}
	if kind == "quota" && failed.ErrorCode != "WORKSPACE_QUOTA_EXCEEDED" {
		return proof, fmt.Errorf("real full filesystem did not report quota: %s", failed.ErrorCode)
	}
	if err := repair(); err != nil {
		return proof, err
	}
	repair = nil
	if _, err := client.JSON(ctx, "p2-fault-resume", "enterprise", http.MethodPost, path+"/resume", 202, map[string]any{"expected_version": failed.Version}, enterpriseHeaders(env, "")); err != nil {
		return proof, err
	}
	after, err := a.waitPlanV2Files(ctx, env, path)
	if err != nil {
		return proof, err
	}
	if err := a.verifyPlanV2FileBytes(ctx, env, convo, after); err != nil {
		return proof, err
	}
	if after.Manifest.AttemptID != before.Manifest.AttemptID {
		return proof, fmt.Errorf("repair refetched a sealed query")
	}
	proof.Facts["same_attempt_after_resume"], proof.Facts["hashes_verified"] = true, true
	return proof, nil
}

func (a *App) planV2DashboardLifecycle(ctx context.Context, env *E2EEnvironment, id, operation string) error {
	client, _ := scenarioHTTP(env)
	value, err := client.JSON(ctx, "p2-fault-dashboard-state", "enterprise", http.MethodGet, "/dashboards/"+id, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	key := "p2-fault-lifecycle-" + uuid.NewString()
	preview, err := client.JSON(ctx, key, "enterprise", http.MethodPost, "/dashboards/"+id+"/actions/preview", 201, planV2LifecycleInput(operation, nestedMap(value, "dashboard")["version"]), enterpriseHeaders(env, key))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	_, err = a.confirmPendingAction(ctx, env, key+"-confirm", ref)
	return err
}

func planV2LifecycleInput(operation string, version any) map[string]any {
	return map[string]any{"operation": operation, "expected_version": version, "name": "", "description": "", "sort_order": 0}
}

func (a *App) corruptPlanV2Chunk(ctx context.Context, env *E2EEnvironment, conversation, job, attempt string) (func() error, error) {
	key, err := a.postgresQuery(ctx, env, "SELECT chunks->0->>'key' FROM dashboard_query_files WHERE job_id='"+job+"' AND attempt_id='"+attempt+"' AND file_kind='data' ORDER BY id LIMIT 1;")
	if err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	prefix := "enterprises/" + env.State.Values["enterprise_id"] + "/conversations/" + conversation + "/dashboard/" + job + "/" + attempt + "/"
	if !strings.HasPrefix(key, prefix) {
		return nil, fmt.Errorf("refusing to corrupt a foreign object")
	}
	forward, err := env.Kube.PortForwardService(ctx, env.SystemNS, "argus-minio", []string{"0:9000"}, io.Discard)
	if err != nil {
		return nil, err
	}
	user, err := dataCredentialValue(ctx, env, "minio-root-user")
	if err != nil {
		_ = forward.Stop()
		return nil, err
	}
	password, err := dataCredentialValue(ctx, env, "minio-root-password")
	if err != nil {
		_ = forward.Stop()
		return nil, err
	}
	client, err := minio.New(fmt.Sprintf("127.0.0.1:%d", forward.Ports[0].Local), &minio.Options{Creds: credentials.NewStaticV4(user, password, ""), Secure: false})
	if err != nil {
		_ = forward.Stop()
		return nil, err
	}
	const bucket = "argus-workspace-files"
	object, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		_ = forward.Stop()
		return nil, err
	}
	original, err := io.ReadAll(io.LimitReader(object, (1<<20)+1))
	_ = object.Close()
	if err != nil {
		_ = forward.Stop()
		return nil, err
	}
	if len(original) > 1<<20 {
		_ = forward.Stop()
		return nil, fmt.Errorf("unexpected chunk exceeds preservation limit")
	}
	if _, err = client.PutObject(ctx, bucket, key, strings.NewReader("corrupt"), 7, minio.PutObjectOptions{}); err != nil {
		_ = forward.Stop()
		return nil, err
	}
	return func() error {
		defer forward.Stop()
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		_, err := client.PutObject(cleanup, bucket, key, bytes.NewReader(original), int64(len(original)), minio.PutObjectOptions{})
		return err
	}, nil
}

const planV2FillQuota = `python - <<'PY'
import errno,os
from pathlib import Path
root=Path('/workspace/p2-quota');root.mkdir(exist_ok=True)
(root/'preserved.txt').write_text('retained')
try:
 for i in range(8):
  with (root/('fill-'+str(i))).open('wb',buffering=0) as out:
   for _ in range(64):out.write(b'x'*1048576)
   os.fsync(out.fileno())
except OSError as e:
 assert e.errno in (errno.ENOSPC,errno.EDQUOT),e
else:raise AssertionError('workspace did not enforce its hard quota')
assert (root/'preserved.txt').read_text()=='retained'
print('real-quota-filled')
PY`
const planV2FreeQuota = `python - <<'PY'
from pathlib import Path
root=Path('/workspace/p2-quota')
for path in root.glob('fill-*'):path.unlink()
assert (root/'preserved.txt').read_text()=='retained'
print('real-quota-released-original-retained')
PY`
