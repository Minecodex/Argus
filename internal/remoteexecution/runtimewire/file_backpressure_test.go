package runtimewire

import (
	"context"
	"io"
	"os/user"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution/runner"
)

type pausedFileReceipt struct {
	writer           io.Writer
	pause            atomic.Bool
	blocked, release chan struct{}
}

func (p *pausedFileReceipt) Write(b []byte) (int, error) {
	if p.pause.CompareAndSwap(true, false) {
		close(p.blocked)
		<-p.release
	}
	return p.writer.Write(b)
}

func TestFileTransferBackpressureDoesNotPreventStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	manager := runner.New(t.TempDir())
	writer := &pausedFileReceipt{writer: outW, blocked: make(chan struct{}), release: make(chan struct{})}
	serverDone := make(chan error, 1)
	go func() { serverDone <- Serve(ctx, manager, inR, writer); outW.Close() }()
	client := NewClient(inW, outR, func() { inR.Close(); outR.Close() }, nil)
	defer func() {
		select {
		case <-writer.release:
		default:
			close(writer.release)
		}
		client.Close()
		<-serverDone
	}()
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	b := &connectorv1.RemoteExecutionBatch{SessionId: uuid.NewString(), SessionGeneration: uuid.NewString(), RequestId: uuid.NewString(), IdentityBinding: uuid.NewString(), ResourceId: uuid.NewString(), ResourceType: "host", ActionRef: "test_approved", ApprovedContentHash: strings.Repeat("a", 64), Username: account.Username, Interpreter: "bash", Sequence: 1, TimeoutSeconds: 30, MaxOutputBytes: 1024}
	if runtime.GOOS == "windows" {
		b.Interpreter = "powershell"
		b.IdentitySid = account.Uid
	}
	if _, err = client.Call(ctx, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "open", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Binding: b}); err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 64<<10)
	const chunks = 128
	for n := 0; n < chunks-1; n++ {
		if _, err = client.Call(ctx, "remote_execution_file", &connectorv1.RemoteExecutionFile{Operation: "write_input", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Path: "large.bin", ByteSize: chunks * uint64(len(chunk)), Offset: uint64(n * len(chunk)), Data: chunk}); err != nil {
			t.Fatal(err)
		}
	}
	writer.pause.Store(true)
	fileDone := make(chan error, 1)
	go func() {
		_, e := client.Call(ctx, "remote_execution_file", &connectorv1.RemoteExecutionFile{Operation: "write_input", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Path: "large.bin", ByteSize: chunks * uint64(len(chunk)), Offset: (chunks - 1) * uint64(len(chunk)), Data: chunk})
		fileDone <- e
	}()
	select {
	case <-writer.blocked:
	case <-ctx.Done():
		t.Fatal("file receipt did not reach the backpressure boundary")
	}
	stopDone := make(chan error, 1)
	go func() {
		receipt, e := client.Call(ctx, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "stop", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId})
		if e == nil && receipt.(*connectorv1.RemoteExecutionControlResult).Status != "stopped" {
			e = io.ErrUnexpectedEOF
		}
		stopDone <- e
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		state, e := manager.Control(ctx, &connectorv1.RemoteExecutionControl{Operation: "status", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId})
		if e != nil {
			t.Fatal(e)
		}
		if state.Status == "lost" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending file receipt blocked actual interpreter termination")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(writer.release)
	for _, done := range []chan error{fileDone, stopDone} {
		select {
		case e := <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("control/file receipts did not reconcile after backpressure")
		}
	}
	if _, err = manager.Open(ctx, b); err == nil {
		t.Fatal("stopped transfer silently reused the interpreter")
	}
}
