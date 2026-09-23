package argusdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) configureP5Workspace(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	secret, err := env.Kube.Client.CoreV1().Secrets(env.SandboxNS).Get(ctx, "argus-opensandbox-api", metav1.GetOptions{})
	if err != nil {
		return err
	}
	headers := map[string]string{"Origin": env.PlatformOrigin(), "X-CSRF-Token": env.State.Values["platform_csrf"], "Idempotency-Key": "p5-workspace-backend-" + env.Options.RunID}
	backend, err := client.JSON(ctx, "p5-workspace-backend", "platform", http.MethodPost, "/platform/sandbox/backends", 201, map[string]any{"name": "P5 Workspace", "endpoint": "http://opensandbox-server." + env.SandboxNS + ".svc", "api_key": string(secret.Data["api-key"]), "status": "enabled", "expected_version": 0}, headers)
	if err != nil {
		return err
	}
	backendID, _ := stringField(backend, "id")
	headers["Idempotency-Key"] = "p5-workspace-backend-health-" + env.Options.RunID
	tested, err := client.JSON(ctx, "p5-workspace-backend-health", "platform", http.MethodPost, "/platform/sandbox/backends/"+backendID+"/test", 200, nil, headers)
	if err != nil {
		return err
	}
	if tested["health_status"] != "healthy" {
		return fmt.Errorf("P5 OpenSandbox health failed")
	}
	registry, err := e2eRegistry(env.ConfigPath)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(registry)
	if err != nil {
		return err
	}
	reference, err := parseRegistryReference("localhost:" + port + "/argus/argus-workspace:" + env.ImageTag)
	if err != nil {
		return err
	}
	digest, found, err := registryManifestDigest(ctx, &http.Client{Timeout: 30 * time.Second}, reference, reference.tag)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("P5 Workspace image digest missing")
	}
	headers["Idempotency-Key"] = "p5-workspace-image-" + env.Options.RunID
	image, err := client.JSON(ctx, "p5-workspace-image", "platform", http.MethodPost, "/platform/sandbox/images", 201, map[string]any{"backend_id": backendID, "name": "P5 offline Python/Node", "image_ref": registry + "/argus/argus-workspace:" + env.ImageTag, "digest": digest, "status": "enabled", "expected_version": 0}, headers)
	if err != nil {
		return err
	}
	imageID, _ := stringField(image, "id")
	headers["Idempotency-Key"] = "p5-workspace-profile-" + env.Options.RunID
	if _, err := client.JSON(ctx, "p5-workspace-profile", "platform", http.MethodPost, "/platform/sandbox/profiles", 201, map[string]any{"name": "P5 offline workspace", "backend_id": backendID, "image_id": imageID, "task_kinds": []string{"agent_workspace"}, "cpu_millis": 500, "memory_mib": 512, "timeout_seconds": 300, "network_mode": "none", "status": "enabled", "expected_version": 0}, headers); err != nil {
		return err
	}
	delete(headers, "Idempotency-Key")
	quotaPath := "/platform/sandbox/enterprise-quotas/" + env.State.Values["enterprise_id"]
	quota, err := client.JSON(ctx, "p5-workspace-quota-current", "platform", http.MethodGet, quotaPath, 200, nil, headers)
	if err != nil {
		return err
	}
	_, err = client.JSON(ctx, "p5-workspace-quota", "platform", http.MethodPut, quotaPath, 200, map[string]any{"max_concurrent_sessions": 4, "monthly_session_seconds": 86400, "expected_version": quota["version"]}, headers)
	return err
}

func (a *App) verifyP5Workspace(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	client.Client("enterprise").Timeout = 6 * time.Minute
	conversation, err := a.p5Conversation(ctx, env, "workspace")
	if err != nil {
		return err
	}
	content := []byte("value\n1\n2\n3\n")
	upload, err := client.JSON(ctx, "p5-upload-create", "enterprise", http.MethodPost, "/conversations/"+conversation+"/workspace/uploads", 201, map[string]any{"name": "business.csv", "byte_size": len(content)}, enterpriseHeaders(env, "p5-upload-create"))
	if err != nil {
		return err
	}
	uploadID, _ := stringField(upload, "id")
	data, _, err := p5FileHTTP(ctx, client, env, http.MethodPut, "/conversations/"+conversation+"/workspace/uploads/"+uploadID+"/content", content, 201, "")
	if err != nil {
		return err
	}
	var file map[string]any
	if json.Unmarshal(data, &file) != nil {
		return fmt.Errorf("P5 upload response is invalid")
	}
	fileID, _ := stringField(file, "id")
	filePath, _ := stringField(file, "path")
	if file["content_hash"] != fmt.Sprintf("%x", sha256.Sum256(content)) {
		return fmt.Errorf("P5 uploaded file hash differs")
	}
	coldStart := time.Now()
	if err := a.p5BashProof(ctx, env, conversation, "network", p5NetworkProof); err != nil {
		return err
	}
	cold := time.Since(coldStart)
	firstCompute, err := a.p5WorkspacePod(ctx, env, conversation)
	if err != nil {
		return err
	}
	pyPath, _ := json.Marshal(filePath)
	command := "python - <<'PY'\nimport pandas as pd\nfrom pathlib import Path\nvalue=pd.read_csv(" + string(pyPath) + ")[\"value\"].sum()\nPath('/workspace/report.txt').write_text(str(value)+'\\n')\nprint('analysis-complete')\nPY"
	run, err := a.p5Run(ctx, env, conversation, "analyse", []p5ToolStep{
		{"bash", map[string]any{"command": command}},
		{"tool.describe", map[string]any{"category": "workflow", "name": "publish_file"}},
		{"tool.invoke", map[string]any{"category": "workflow", "name": "publish_file", "arguments": map[string]any{"path": "report.txt", "name": "report.txt", "media_type": "text/plain"}}},
		{"write", map[string]any{"path": "report.txt", "content": "changed after publication"}},
	}, []string{fileID})
	if err != nil {
		return err
	}
	delivery, err := a.postgresQuery(ctx, env, "SELECT id::text FROM file_deliveries WHERE run_id='"+run+"';")
	if err != nil {
		return err
	}
	delivery = strings.TrimSpace(delivery)
	if delivery == "" {
		return fmt.Errorf("P5 analysis did not publish an immutable file")
	}
	path := "/conversations/" + conversation + "/deliveries/" + delivery + "/content"
	downloaded, header, err := p5FileHTTP(ctx, client, env, http.MethodGet, path, nil, 200, "")
	if err != nil {
		return err
	}
	if string(downloaded) != "6\n" || header.Get("ETag") != `"sha256:`+fmt.Sprintf("%x", sha256.Sum256(downloaded))+`"` {
		return fmt.Errorf("P5 published delivery changed with the working file")
	}
	ranged, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, path, nil, 206, "bytes=0-0")
	if err != nil {
		return err
	}
	if string(ranged) != "6" {
		return fmt.Errorf("P5 Range download differs")
	}
	env.State.Values["p5_workspace_conversation_id"] = conversation
	if err := a.verifyP5WarmReuse(ctx, env, conversation, cold, firstCompute); err != nil {
		return err
	}
	if err := a.p5BashProof(ctx, env, conversation, "background-writer", p5BackgroundWriter); err != nil {
		return err
	}
	if err := a.p5BashProof(ctx, env, conversation, "writer-revocation", p5WriterRevocation); err != nil {
		return err
	}
	if err := a.p5BashProof(ctx, env, conversation, "hard-quota", p5HardQuotaProof); err != nil {
		return err
	}
	after, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+fileID+"/content", nil, 200, "")
	if err != nil {
		return err
	}
	if !bytes.Equal(after, content) {
		return fmt.Errorf("P5 quota pressure destroyed an existing upload")
	}
	if err := a.verifyP5ComputeRecoveryAndIdle(ctx, env, conversation); err != nil {
		return err
	}
	if err := a.verifyP5WorkspaceControlFaults(ctx, env, conversation); err != nil {
		return err
	}
	modelOnly, err := a.p5Conversation(ctx, env, "model-only-files")
	if err != nil {
		return err
	}
	if err := a.p5BashProof(ctx, env, modelOnly, "model-only-files", "printf generated > /workspace/generated.txt"); err != nil {
		return err
	}
	metadata, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM workspace_files f JOIN workspaces w ON w.id=f.workspace_id WHERE w.conversation_id='"+modelOnly+"';")
	if err != nil || strings.TrimSpace(metadata) != "0" {
		return fmt.Errorf("P5 model-only fixture unexpectedly has uploaded file metadata: %v", err)
	}
	if err := a.runPlaywright(ctx, env, "e2e/p5-real.spec.ts", map[string]string{
		"ARGUS_P5_E2E": "1", "ARGUS_P5_USERNAME": env.State.Values["enterprise_username"], "ARGUS_P5_PASSWORD": env.State.Values["enterprise_password"],
		"ARGUS_P5_NATIVE_CONVERSATION": env.State.Values["p5_native_conversation_id"], "ARGUS_P5_WORKSPACE_CONVERSATION": conversation, "ARGUS_P5_CAPACITY_CONVERSATION": env.State.Values["p5_capacity_conversation_id"],
		"ARGUS_P5_MODEL_ONLY_CONVERSATION":         modelOnly,
		"ARGUS_P5_COMPACTION_FAILURE_CONVERSATION": env.State.Values["p5_compaction_failure_conversation"],
	}); err != nil {
		return err
	}
	before, err := a.p5WorkspaceContextProbe(ctx, env, conversation, "before-delete", []string{fileID})
	if err != nil {
		return err
	}
	if status, _ := nestedString(before, "workspace", "status"); status != "ready" {
		return fmt.Errorf("P5 current workspace was not visible to provider")
	}
	if _, err := client.JSON(ctx, "p5-workspace-explicit-delete", "enterprise", http.MethodDelete, "/conversations/"+conversation+"/workspace", 202, nil, enterpriseHeaders(env, "p5-delete")); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		state, err := a.postgresQuery(ctx, env, "SELECT status FROM workspaces WHERE conversation_id='"+conversation+"' ORDER BY created_at DESC LIMIT 1;")
		if err != nil {
			return err
		}
		if strings.TrimSpace(state) == "deleted" {
			break
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
	status, err := a.postgresQuery(ctx, env, "SELECT status FROM workspaces WHERE conversation_id='"+conversation+"' ORDER BY created_at DESC LIMIT 1;")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "deleted" {
		return fmt.Errorf("P5 Workspace cleanup did not finish")
	}
	_, _, err = p5FileHTTP(ctx, client, env, http.MethodGet, path, nil, 404, "")
	if err != nil {
		return err
	}
	return a.verifyP5DeletedWorkspaceContext(ctx, env, conversation, before)
}

func (a *App) p5BashProof(ctx context.Context, env *E2EEnvironment, conversation, name, command string) error {
	run, err := a.p5Run(ctx, env, conversation, name, []p5ToolStep{{"bash", map[string]any{"command": command, "timeout_seconds": 120}}}, []string{})
	if err != nil {
		return err
	}
	value, err := a.postgresQuery(ctx, env, "SELECT projection->'summary'->>'exit_code' FROM tool_results r JOIN tool_calls t ON t.id=r.tool_call_id WHERE t.run_id='"+run+"' AND t.tool_id='bash';")
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) != "0" {
		if name == "network" {
			reasons := map[string]string{"11": "public IPv4 HTTPS reachable", "12": "DNS query was not rejected", "13": "metadata endpoint reachable", "14": "cluster service reachable", "15": "public IPv6 reachable", "21": "DNS name resolution allowed", "22": "user code is root", "30": "database environment exposed", "31": "egress policy token environment exposed", "32": "object storage environment exposed", "41": "file RPC private key readable"}
			if reason := reasons[strings.TrimSpace(value)]; reason != "" {
				return fmt.Errorf("P5 network proof failed: %s", reason)
			}
		}
		return fmt.Errorf("P5 %s proof failed: exit code %s", name, value)
	}
	return nil
}
func p5FileHTTP(ctx context.Context, client *ScenarioHTTP, env *E2EEnvironment, method, path string, body []byte, expected int, requestedRange string) ([]byte, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, method, client.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	for key, value := range enterpriseHeaders(env, "") {
		request.Header.Set(key, value)
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	if requestedRange != "" {
		request.Header.Set("Range", requestedRange)
	}
	response, err := client.Client("enterprise").Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 101<<20))
	if err != nil {
		return nil, nil, err
	}
	if response.StatusCode != expected {
		return nil, response.Header, fmt.Errorf("P5 file %s %s returned %d, want %d: %s", method, path, response.StatusCode, expected, string(data[:min(len(data), 1024)]))
	}
	return data, response.Header, nil
}
func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

const p5NetworkProof = `python - <<'PY'
import os, socket, struct
targets=[(11,'1.1.1.1',443),(13,'169.254.169.254',80),(14,os.environ.get('KUBERNETES_SERVICE_HOST','10.96.0.1'),443),(15,'2606:4700:4700::1111',443)]
for code,host,port in targets:
 try:
  s=socket.create_connection((host,port),timeout=.5)
 except OSError: pass
 else:
  s.close();raise SystemExit(code)
# Port 53 is redirected to the local deny-policy proxy. Test DNS semantics
# instead of treating a local proxy TCP handshake as outside connectivity.
query=struct.pack('!HHHHHH',0x5035,0x0100,1,0,0,0)+b'\x07example\x03com\x00'+struct.pack('!HH',1,1)
for kind in [socket.SOCK_DGRAM,socket.SOCK_STREAM]:
 try:
  with socket.socket(socket.AF_INET,kind) as dns:
   dns.settimeout(1)
   if kind==socket.SOCK_DGRAM:
    dns.sendto(query,('8.8.8.8',53));reply=dns.recv(4096)
   else:
    dns.connect(('8.8.8.8',53));dns.sendall(struct.pack('!H',len(query))+query)
    prefix=dns.recv(2)
    if len(prefix)!=2: raise SystemExit(12)
    size=struct.unpack('!H',prefix)[0];reply=b''
    while len(reply)<size:
     part=dns.recv(size-len(reply))
     if not part: raise SystemExit(12)
     reply+=part
   if len(reply)<12: raise SystemExit(12)
   ident,flags,questions,answers,authority,extra=struct.unpack('!HHHHHH',reply[:12])
   if ident!=0x5035 or flags&0x8000==0 or flags&15 not in (3,5) or answers!=0: raise SystemExit(12)
 except OSError: pass
try: socket.getaddrinfo('example.com',443)
except OSError: pass
else: raise SystemExit(21)
if os.getuid()<100000: raise SystemExit(22)
for index,variable in enumerate(['ARGUS_DATABASE_URL','OPENSANDBOX_EGRESS_TOKEN','ARGUS_OBJECT_STORE_SECRET_KEY']):
 if variable in os.environ: raise SystemExit(30+index)
for path in ['/var/run/argus/workspace/tls.key','/var/run/argus/manager/tls/tls.key','/proc/1/environ']:
 try: open(path,'rb').read()
 except OSError: pass
 else: raise SystemExit(41)
for line in open('/proc/self/status'):
 if line.startswith(('CapInh:','CapPrm:','CapEff:','CapAmb:')) and int(line.split()[1],16): raise SystemExit(42)
print('network-isolation-proved')
PY`

const p5HardQuotaProof = `python - <<'PY'
import errno,multiprocessing as mp,os
from pathlib import Path
old=Path('/workspace/preserved.txt');old.write_text('preserved')
def fill(i,q):
 try:
  with open('/workspace/fill-'+str(i),'wb',buffering=0) as f:
   while True: f.write(b'x'*1048576);os.fsync(f.fileno())
 except OSError as e: q.put(e.errno)
q=mp.Queue();processes=[mp.Process(target=fill,args=(i,q)) for i in range(4)]
for p in processes:p.start()
for p in processes:p.join(60);assert not p.is_alive()
assert all(q.get(timeout=5) in (errno.ENOSPC,errno.EDQUOT) for _ in processes)
assert old.read_text()=='preserved'
for i in range(4):Path('/workspace/fill-'+str(i)).unlink()
assert old.read_text()=='preserved'
print('hard-quota-and-retention-proved')
PY`

const p5BackgroundWriter = `python - <<'PY'
import subprocess,time
from pathlib import Path
path=Path('/workspace/retired-writer.log')
path.unlink(missing_ok=True)
script="import time; f=open('/workspace/retired-writer.log','ab',buffering=0);\nwhile True: f.write(b'x'); time.sleep(.05)"
subprocess.Popen(['python','-c',script],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,start_new_session=True)
for _ in range(100):
 if path.exists() and path.stat().st_size: break
 time.sleep(.02)
assert path.exists() and path.stat().st_size
print('background-writer-started')
PY`

const p5WriterRevocation = `python - <<'PY'
import time
from pathlib import Path
path=Path('/workspace/retired-writer.log')
before=path.read_bytes()
time.sleep(1)
assert path.read_bytes()==before, 'old process still writes after Workspace handoff'
print('old-writer-revocation-proved')
PY`
