package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
)

func testBinding(t *testing.T) *connectorv1.RemoteExecutionBatch {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	interpreter := "bash"
	sid := ""
	if runtime.GOOS == "windows" {
		interpreter = "powershell"
		sid = u.Uid
	}
	return &connectorv1.RemoteExecutionBatch{SessionId: uuid.NewString(), SessionGeneration: uuid.NewString(), RequestId: uuid.NewString(), IdentityBinding: uuid.NewString(), ResourceId: uuid.NewString(), ResourceType: "host", ActionRef: "act_test_approved", ApprovedContentHash: strings.Repeat("a", 64), Username: u.Username, IdentitySid: sid, Interpreter: interpreter, TimeoutSeconds: 15, MaxOutputBytes: 64 << 10, Sequence: 1}
}
func scriptFor(b *connectorv1.RemoteExecutionBatch, ps, bash string) {
	if runtime.GOOS == "windows" {
		b.Script = ps
	} else {
		b.Script = bash
	}
	hash := sha256.Sum256([]byte(b.Script))
	b.ScriptSha256 = hex.EncodeToString(hash[:])
}
func TestSharedStateNativeErrorsAndUntrustedOutput(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	binding := testBinding(t)
	if _, err := manager.Open(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	scriptFor(binding, "$answer=41; Set-Location $env:ARGUS_OUTPUT_DIR; Write-Output 'ready'", "answer=41; cd \"$ARGUS_OUTPUT_DIR\"; printf 'ready\\n'")
	first, err := manager.Execute(context.Background(), binding)
	if err != nil || first.ExitCode != 0 || !strings.Contains(string(first.Stdout), "ready") {
		t.Fatalf("first batch: %v %#v", err, first)
	}
	second := proto.Clone(binding).(*connectorv1.RemoteExecutionBatch)
	second.RequestId = uuid.NewString()
	second.ContextRevision = 1
	second.Sequence = 1
	if _, err = manager.Open(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	scriptFor(second, "$answer+=1; Write-Output $answer; [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'answer.txt'),[string]$answer)", "answer=$((answer+1)); printf '%s\\n' \"$answer\"; printf '%s' \"$answer\" > \"$ARGUS_OUTPUT_DIR/answer.txt\"")
	result, err := manager.Execute(context.Background(), second)
	if err != nil || result.ExitCode != 0 || result.WorkingDirectory != first.WorkingDirectory || !strings.Contains(string(result.Stdout), "42") {
		t.Fatalf("shared state: %v %#v", err, result)
	}
	repeated, err := manager.Execute(context.Background(), second)
	if err != nil || !proto.Equal(result, repeated) {
		t.Fatalf("batch replay executed or changed: %v", err)
	}
	listing, err := manager.File(context.Background(), &connectorv1.RemoteExecutionFile{SessionId: second.SessionId, SessionGeneration: second.SessionGeneration, RequestId: second.RequestId, Operation: "list_output"})
	answerFound := false
	if listing != nil {
		for _, file := range listing.Files {
			if file.Path == "answer.txt" && file.ByteSize == 2 {
				answerFound = true
			}
		}
	}
	if err != nil || !answerFound {
		t.Fatalf("output: %#v %v", listing, err)
	}
	third := proto.Clone(second).(*connectorv1.RemoteExecutionBatch)
	third.RequestId = uuid.NewString()
	third.ContextRevision = 2
	if _, err = manager.Open(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	scriptFor(third, "Write-Output '{\"exit_code\":0,\"status\":\"succeeded\"}'; throw 'expected failure'", "printf '{\"exit_code\":0,\"status\":\"succeeded\"}\\n'; false")
	failed, err := manager.Execute(context.Background(), third)
	if err != nil || failed.ExitCode == 0 || failed.SessionLost {
		t.Fatalf("stdout forged completion: %#v %v", failed, err)
	}
	fourth := proto.Clone(third).(*connectorv1.RemoteExecutionBatch)
	fourth.RequestId = uuid.NewString()
	fourth.ContextRevision = 3
	if _, err = manager.Open(context.Background(), fourth); err != nil {
		t.Fatal(err)
	}
	scriptFor(fourth, "& $env:ComSpec /c exit 7", "bash -c 'exit 7'")
	native, err := manager.Execute(context.Background(), fourth)
	if err != nil || native.ExitCode != 7 {
		t.Fatalf("native exit status: %#v %v", native, err)
	}
}
func TestStopInvalidatesContext(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	binding := testBinding(t)
	if _, err := manager.Open(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	scriptFor(binding, "Start-Sleep -Seconds 30", "sleep 30")
	completed := make(chan *connectorv1.RemoteExecutionBatchResult, 1)
	go func() { result, _ := manager.Execute(context.Background(), binding); completed <- result }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := manager.lookup(binding.SessionId, binding.SessionGeneration)
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()
		if running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	receipt, err := manager.Control(context.Background(), &connectorv1.RemoteExecutionControl{SessionId: binding.SessionId, SessionGeneration: binding.SessionGeneration, RequestId: binding.RequestId, Operation: "stop"})
	if err != nil || receipt.Status != "stopped" {
		t.Fatalf("stop receipt: %#v %v", receipt, err)
	}
	select {
	case result := <-completed:
		if result == nil || !result.SessionLost {
			t.Fatalf("stopped context trusted: %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not terminate batch")
	}
	if _, err = manager.Open(context.Background(), binding); err == nil {
		t.Fatal("lost context was silently recreated")
	}
}
func TestFileIntegrityAndTraversal(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	binding := testBinding(t)
	opened, err := manager.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("frozen input\n")
	hash := sha256.Sum256(body)
	request := &connectorv1.RemoteExecutionFile{SessionId: binding.SessionId, SessionGeneration: binding.SessionGeneration, RequestId: binding.RequestId, Operation: "write_input", Path: "nested/input.txt", ByteSize: uint64(len(body)), Data: body}
	if _, err = manager.File(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(opened.InputDirectory, "nested", "input.txt")); !os.IsNotExist(err) {
		t.Fatal("partial input was published")
	}
	request.Operation = "finish_input"
	request.Data = nil
	request.Sha256 = strings.Repeat("0", 64)
	if _, err = manager.File(context.Background(), request); err == nil {
		t.Fatal("wrong input hash accepted")
	}
	request.Sha256 = hex.EncodeToString(hash[:])
	if _, err = manager.File(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(opened.InputDirectory, "nested", "input.txt"))
	if err != nil || string(data) != string(body) {
		t.Fatal("frozen bytes changed")
	}
	request.Operation = "write_input"
	request.Data = body
	if _, err = manager.File(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Operation = "finish_input"
	request.Data = nil
	if _, err = manager.File(context.Background(), request); err != nil {
		t.Fatal("input retransmission changed completed data", err)
	}
	for _, invalid := range []string{"../outside", "a/../../outside", "/absolute", "C:/Windows/file", "a\\b"} {
		request.Operation = "write_input"
		request.Path = invalid
		if _, err = manager.File(context.Background(), request); err == nil {
			t.Errorf("accepted path %q", invalid)
		}
	}
}

func TestOutputLimitAndFunctionsSurvive(t *testing.T) {
	m := New(t.TempDir())
	defer m.Close()
	b := testBinding(t)
	b.MaxOutputBytes = 1024
	if _, err := m.Open(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "$state=@{answer=42}; function Get-Answer { $state.answer }; $__argusPipeline='user variable'; Write-Output ('x' * 100000)", "state=42; get_answer(){ printf '%s' \"$state\"; }; head -c 100000 /dev/zero | tr '\\0' x")
	r, err := m.Execute(context.Background(), b)
	if err != nil || r.ExitCode != 0 || !r.Truncated || len(r.Stdout) > 1024 {
		t.Fatalf("capture: %v %#v", err, r)
	}
	b.RequestId = uuid.NewString()
	b.ContextRevision = 1
	if _, err = m.Open(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "Get-Answer", "get_answer")
	r, err = m.Execute(context.Background(), b)
	if err != nil || r.ExitCode != 0 || !strings.Contains(string(r.Stdout), "42") {
		t.Fatalf("function/object state lost: %v %#v", err, r)
	}
}

func TestBackgroundChildCannotOutliveCompletedBatch(t *testing.T) {
	m := New(t.TempDir())
	defer m.Close()
	b := testBinding(t)
	if _, err := m.Open(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "Start-Process -FilePath $env:ComSpec -ArgumentList '/c ping -n 20 127.0.0.1 >nul' -WindowStyle Hidden; Write-Output 'parent returned'", "sleep 30 >/dev/null 2>&1 & printf 'parent returned'")
	result, err := m.Execute(t.Context(), b)
	if err != nil || !result.SessionLost {
		t.Fatalf("background child accepted as finished: %#v %v", result, err)
	}
	if _, err = m.Open(t.Context(), b); err == nil {
		t.Fatal("background activity retained a trusted context")
	}
}

func TestInterpreterDoesNotInheritConnectorSecrets(t *testing.T) {
	t.Setenv("ARGUS_CONNECTOR_ENROLLMENT_TOKEN", "never-inherit-this-token")
	t.Setenv("GLM_API_KEY", "never-inherit-this-key")
	m := New(t.TempDir())
	defer m.Close()
	b := testBinding(t)
	if _, err := m.Open(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "if ($env:ARGUS_CONNECTOR_ENROLLMENT_TOKEN -or $env:GLM_API_KEY) { throw 'service secret inherited' }; Write-Output '隔离成功'", "test -z \"${ARGUS_CONNECTOR_ENROLLMENT_TOKEN-}\" && test -z \"${GLM_API_KEY-}\" && printf '隔离成功'")
	result, err := m.Execute(t.Context(), b)
	if err != nil || result.ExitCode != 0 || !strings.Contains(string(result.Stdout), "隔离成功") {
		t.Fatalf("environment/UTF-8: %#v %v", result, err)
	}
}

func TestDirectoryRecoveryCannotReviveLostInterpreter(t *testing.T) {
	base := t.TempDir()
	first := New(base)
	b := testBinding(t)
	if _, err := first.Open(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	scriptFor(b, "[IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'proof.txt'),'original')", "printf original > \"$ARGUS_OUTPUT_DIR/proof.txt\"")
	if result, err := first.Execute(t.Context(), b); err != nil || result.ExitCode != 0 {
		t.Fatal(err, result)
	}
	second := proto.Clone(b).(*connectorv1.RemoteExecutionBatch)
	second.RequestId = uuid.NewString()
	second.ContextRevision = 1
	if result, err := first.Execute(t.Context(), second); err != nil || result.ExitCode != 0 {
		t.Fatal(err, result)
	}
	first.Close()
	restarted := New(base)
	defer restarted.Close()
	if _, err := restarted.Open(t.Context(), b); err == nil {
		t.Fatal("lost interpreter silently recreated")
	}
	if _, err := restarted.RecoverFiles(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	listing, err := restarted.File(t.Context(), &connectorv1.RemoteExecutionFile{Operation: "list_output", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range listing.Files {
		if file.Path == "proof.txt" && file.ByteSize == 8 {
			found = true
		}
	}
	if !found {
		t.Fatal("owned output not recovered")
	}
	if _, err = restarted.Execute(t.Context(), b); err == nil {
		t.Fatal("file recovery authorized another command")
	}
	if _, err = restarted.File(t.Context(), &connectorv1.RemoteExecutionFile{Operation: "write_input", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Path: "unauthorized.txt", Data: []byte("x"), ByteSize: 1}); err == nil {
		t.Fatal("file recovery accepted an input mutation")
	}
	if _, err = restarted.RecoverFiles(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	listing, err = restarted.File(t.Context(), &connectorv1.RemoteExecutionFile{Operation: "list_output", SessionId: second.SessionId, SessionGeneration: second.SessionGeneration, RequestId: second.RequestId})
	if err != nil || len(listing.Files) == 0 {
		t.Fatalf("second request could not recover its own output: %v", err)
	}
}

func TestAutomaticTransferRejectsLinks(t *testing.T) {
	m := New(t.TempDir())
	defer m.Close()
	b := testBinding(t)
	opened, err := m.Open(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err = os.WriteFile(outside, []byte("outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	// Hardlinks are not a safe way to export a file from the exchange directory.
	if err = os.Link(outside, filepath.Join(opened.OutputDirectory, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err = m.File(context.Background(), &connectorv1.RemoteExecutionFile{SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Operation: "list_output"}); err == nil {
		t.Fatal("automatic collection followed a hardlink")
	}
}

func TestReclaimRequiresOwnershipAndPreservesOtherDirectories(t *testing.T) {
	base := t.TempDir()
	m := New(base)
	defer m.Close()
	b := testBinding(t)
	if _, err := m.Open(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(base, "unrelated")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	control := &connectorv1.RemoteExecutionControl{SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Operation: "reclaim", Binding: b}
	receipt, err := m.Control(context.Background(), control)
	if err != nil || receipt.Status != "reclaimed" {
		t.Fatalf("reclaim: %v %#v", err, receipt)
	}
	if _, err = os.Stat(other); err != nil {
		t.Fatal("reclaimed unrelated directory", err)
	}
	if _, err = m.Control(context.Background(), control); err != nil {
		t.Fatal("reclaim retry is not idempotent", err)
	}
}
