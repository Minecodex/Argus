//go:build linux

package workspacesupervisor

import (
	"context"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "--child" {
		limit, _ := strconv.Atoi(os.Args[2])
		if Child(limit, os.Args[3]) != nil {
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

func TestDedicatedUIDProcessCleanupAndWarmFence(t *testing.T) {
	if os.Getenv("ARGUS_SUPERVISOR_ISOLATION_TEST") != "1" {
		t.Skip("requires the isolated Linux compute image and dedicated manager UID")
	}
	server, err := New("owned-workspace", 1, 100000, 256)
	if err != nil {
		t.Fatal(err)
	}
	key := "/var/run/argus/manager/tls/test.key"
	if err := os.WriteFile(key, []byte("supervisor-only-test-secret"), 0440); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(key, -1, 100000); err != nil {
		t.Fatal(err)
	}
	authority := &workspacev1.Authority{WorkspaceId: "owned-workspace", FenceToken: 1}
	run := func(command string) *workspacev1.ExecuteResponse {
		t.Helper()
		result, err := server.Execute(context.Background(), &workspacev1.ExecuteRequest{Authority: authority, Command: command, TimeoutSeconds: 10})
		if err != nil || result.ExitCode != 0 {
			t.Fatalf("command failed: %v %v", result, err)
		}
		return result
	}
	run(`python - <<'PY'
import os
assert os.geteuid()==100000
assert not any(k.startswith('ARGUS_') for k in os.environ)
status=open('/proc/self/status').read()
for field in ['CapEff','CapPrm','CapInh','CapAmb']:
 assert int(next(x.split(':')[1].strip() for x in status.splitlines() if x.startswith(field+':')),16)==0
assert 'NoNewPrivs:\t1' in status
for path in ['/var/run/argus/manager/tls/test.key','/proc/1/environ']:
 try: open(path).read()
 except PermissionError: pass
 else: raise AssertionError('manager data accessible')
PY`)
	run(`python - <<'PY'
import subprocess,sys,time
subprocess.Popen([sys.executable,'-c',"import time; f=open('/workspace/orphan','ab',buffering=0);\nwhile True: f.write(b'x'); time.sleep(.01)"],start_new_session=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
time.sleep(.2)
PY`)
	time.Sleep(100 * time.Millisecond)
	before, _ := os.ReadFile("/workspace/orphan")
	if len(before) == 0 {
		t.Fatal("background writer fixture never wrote data")
	}
	time.Sleep(100 * time.Millisecond)
	after, _ := os.ReadFile("/workspace/orphan")
	if len(before) != len(after) {
		t.Fatal("detached writer survived command completion")
	}
	if _, err := server.ReleaseLease(context.Background(), &workspacev1.WorkspaceSupervisorServiceReleaseLeaseRequest{Authority: authority}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.BindLease(context.Background(), &workspacev1.WorkspaceSupervisorServiceBindLeaseRequest{WorkspaceId: "owned-workspace", Generation: 1, FenceToken: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Execute(context.Background(), &workspacev1.ExecuteRequest{Authority: authority, Command: "touch /workspace/stale", TimeoutSeconds: 1}); err == nil {
		t.Fatal("old lease executed")
	}
	authority.FenceToken = 2
	if result := run("printf warm-reuse"); !strings.Contains(result.Stdout, "warm-reuse") {
		t.Fatal("warm instance was unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := server.Execute(ctx, &workspacev1.ExecuteRequest{Authority: authority, Command: "sleep 99 & wait", TimeoutSeconds: 10}); err == nil {
		t.Fatal("cancelled command succeeded")
	}
	if err := sweepUserProcesses(context.Background(), 100000); err != nil {
		t.Fatal(err)
	}
}
