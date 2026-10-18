package runtimewire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os/user"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution/runner"
)

func TestPrivateStreamMultiplexesStopWhileBatchIsRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	commandsRead, commandsWrite := io.Pipe()
	repliesRead, repliesWrite := io.Pipe()
	m := runner.New(t.TempDir())
	serverDone := make(chan error, 1)
	go func() { serverDone <- Serve(ctx, m, commandsRead, repliesWrite); repliesWrite.Close() }()
	client := NewClient(commandsWrite, repliesRead, func() { commandsRead.Close(); repliesRead.Close() }, nil)
	defer func() { client.Close(); <-serverDone }()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	b := &connectorv1.RemoteExecutionBatch{RequestId: uuid.NewString(), SessionId: uuid.NewString(), SessionGeneration: uuid.NewString(), IdentityBinding: uuid.NewString(), ResourceId: uuid.NewString(), ResourceType: "host", ActionRef: "approved", ApprovedContentHash: strings.Repeat("a", 64), Username: u.Username, Interpreter: "bash", TimeoutSeconds: 30, MaxOutputBytes: 1024, Sequence: 1, Script: "sleep 30"}
	if runtime.GOOS == "windows" {
		b.Interpreter = "powershell"
		b.IdentitySid = u.Uid
		b.Script = "Start-Sleep -Seconds 30"
	}
	hash := sha256.Sum256([]byte(b.Script))
	b.ScriptSha256 = hex.EncodeToString(hash[:])
	if _, err = client.Call(ctx, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "open", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Binding: b}); err != nil {
		t.Fatal(err)
	}
	completed := make(chan error, 1)
	go func() { _, err := client.Call(ctx, "remote_execution_batch", b); completed <- err }()
	// A separate control message must be processed while Execute is blocked.
	time.Sleep(150 * time.Millisecond)
	result, err := client.Call(ctx, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "stop", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId})
	if err != nil || result.(*connectorv1.RemoteExecutionControlResult).Status != "stopped" {
		t.Fatalf("stop: %v %#v", err, result)
	}
	select {
	case err = <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("batch blocked stop")
	}
}
