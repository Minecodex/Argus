package connector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/connectorprotocol"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	remoterunner "github.com/kakj-go/Argus/internal/remoteexecution/runner"
	"github.com/kakj-go/Argus/internal/sshtarget"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type runtimeMessage struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}
type runtimeReply struct {
	payload json.RawMessage
	err     error
}
type sshRuntimeBridge struct {
	client  *ssh.Client
	session *ssh.Session
	input   io.WriteCloser
	mu      sync.Mutex
	pending map[string]chan runtimeReply
	writeMu sync.Mutex
	closed  bool
}
type remoteRuntimeHub struct {
	local      *remoterunner.Manager
	mu         sync.Mutex
	bridges    map[string]*sshRuntimeBridge
	kubernetes map[string]*kubernetesRuntimeBridge
}

func newRemoteRuntimeHub() *remoteRuntimeHub {
	return &remoteRuntimeHub{local: remoterunner.New(os.Getenv("ARGUS_REMOTE_DIRECTORY")), bridges: map[string]*sshRuntimeBridge{}, kubernetes: map[string]*kubernetesRuntimeBridge{}}
}
func (h *remoteRuntimeHub) Close() {
	h.local.Close()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, bridge := range h.bridges {
		bridge.close()
	}
	for _, bridge := range h.kubernetes {
		bridge.Close()
	}
}
func (h *remoteRuntimeHub) call(ctx context.Context, kind string, payload proto.Message, credential []byte) (proto.Message, error) {
	key := ""
	var binding *connectorv1.RemoteExecutionBatch
	switch p := payload.(type) {
	case *connectorv1.RemoteExecutionBatch:
		key = p.SessionId + "/" + p.SessionGeneration
	case *connectorv1.RemoteExecutionControl:
		key = p.SessionId + "/" + p.SessionGeneration
		if p.Operation == "open" || p.Operation == "reclaim" || p.Operation == "recover_files" {
			binding = p.Binding
		}
	case *connectorv1.RemoteExecutionFile:
		key = p.SessionId + "/" + p.SessionGeneration
	default:
		return nil, errors.New("REMOTE_INPUT_INVALID")
	}
	h.mu.Lock()
	bridge := h.bridges[key]
	kubeBridge := h.kubernetes[key]
	h.mu.Unlock()
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok && (control.Operation == "recover_files" || control.Operation == "reclaim") && bridge != nil {
		bridge.mu.Lock()
		closed := bridge.closed
		bridge.mu.Unlock()
		if closed {
			h.mu.Lock()
			delete(h.bridges, key)
			h.mu.Unlock()
			bridge = nil
		}
	}
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok && (control.Operation == "recover_files" || control.Operation == "reclaim") && kubeBridge != nil && kubeBridge.Closed() {
		h.mu.Lock()
		if h.kubernetes[key] == kubeBridge {
			delete(h.kubernetes, key)
		}
		h.mu.Unlock()
		kubeBridge = nil
	}
	if binding != nil && binding.ResourceType == "kubernetes_cluster" && kubeBridge == nil {
		var err error
		control, _ := payload.(*connectorv1.RemoteExecutionControl)
		if control != nil && (control.Operation == "recover_files" || control.Operation == "reclaim") {
			kubeBridge, err = recoverKubernetesRuntime(ctx, binding, credential)
		} else {
			kubeBridge, err = openKubernetesRuntime(ctx, binding, credential)
		}
		if err != nil {
			return nil, err
		}
		h.mu.Lock()
		if old := h.kubernetes[key]; old != nil {
			kubeBridge.Close()
			kubeBridge = old
		} else {
			h.kubernetes[key] = kubeBridge
		}
		h.mu.Unlock()
	}
	if kubeBridge != nil {
		result, err := kubeBridge.Call(ctx, kind, payload)
		if reclaimedRemoteRuntime(payload, result, err) {
			h.mu.Lock()
			delete(h.kubernetes, key)
			h.mu.Unlock()
			kubeBridge.Close()
		}
		return result, err
	}
	if binding != nil && binding.ResourceType == "host" {
		current, err := user.Current()
		if err != nil {
			return nil, err
		}
		isLocal := binding.SshAddress == "" && strings.EqualFold(current.Username, binding.Username) && (runtime.GOOS != "windows" || strings.EqualFold(current.Uid, binding.IdentitySid))
		if !isLocal {
			if bridge == nil {
				bridge, err = openSSHRuntime(ctx, binding, credential)
				if err != nil {
					return nil, err
				}
				h.mu.Lock()
				if old := h.bridges[key]; old != nil {
					bridge.close()
					bridge = old
				} else {
					h.bridges[key] = bridge
				}
				h.mu.Unlock()
			}
		}
	}
	if bridge != nil {
		result, err := bridge.call(ctx, kind, payload)
		if reclaimedRemoteRuntime(payload, result, err) {
			h.mu.Lock()
			delete(h.bridges, key)
			h.mu.Unlock()
			bridge.close()
		}
		return result, err
	}
	switch p := payload.(type) {
	case *connectorv1.RemoteExecutionBatch:
		return h.local.Execute(ctx, p)
	case *connectorv1.RemoteExecutionControl:
		return h.local.Control(ctx, p)
	case *connectorv1.RemoteExecutionFile:
		return h.local.File(ctx, p)
	}
	return nil, errors.New("REMOTE_INPUT_INVALID")
}
func reclaimedRemoteRuntime(request, result proto.Message, err error) bool {
	control, ok := request.(*connectorv1.RemoteExecutionControl)
	if !ok || err != nil || control.Operation != "reclaim" {
		return false
	}
	receipt, ok := result.(*connectorv1.RemoteExecutionControlResult)
	return ok && receipt.Status == "reclaimed" && receipt.SessionGeneration == control.SessionGeneration
}
func openSSHRuntime(ctx context.Context, b *connectorv1.RemoteExecutionBatch, credential []byte) (*sshRuntimeBridge, error) {
	if b.SshAddress == "" || b.SshPort == 0 || b.SshHostKey == "" || len(credential) == 0 {
		return nil, errors.New("REMOTE_IDENTITY_UNAVAILABLE")
	}
	auth, err := connectorSSHAuth(credential)
	if err != nil {
		return nil, errors.New("REMOTE_IDENTITY_UNAVAILABLE")
	}
	address := net.JoinHostPort(b.SshAddress, fmtPort(b.SshPort))
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, errors.New("REMOTE_ROUTE_UNAVAILABLE")
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	config := &ssh.ClientConfig{User: b.Username, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if ssh.FingerprintSHA256(key) != b.SshHostKey {
			return errors.New("REMOTE_HOST_KEY_CHANGED")
		}
		return nil
	}}
	clientConn, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		conn.Close()
		return nil, errors.New("REMOTE_ROUTE_UNAVAILABLE")
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(clientConn, channels, requests)
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, errors.New("REMOTE_ROUTE_UNAVAILABLE")
	}
	input, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	output, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	session.Stderr = io.Discard
	command := "/usr/local/bin/argus-connector remote-runtime"
	if b.Interpreter == "powershell" {
		command = sshtarget.PowerShellCommand(`& 'C:\Program Files\Argus\Connector\argus-connector.exe' remote-runtime`)
	}
	if err = session.Start(command); err != nil {
		session.Close()
		client.Close()
		return nil, errors.New("REMOTE_ROUTE_UNAVAILABLE")
	}
	bridge := &sshRuntimeBridge{client: client, session: session, input: input, pending: map[string]chan runtimeReply{}}
	go bridge.read(output)
	return bridge, nil
}
func fmtPort(n uint32) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(digits[n%10]) + out
		n /= 10
	}
	return out
}
func (b *sshRuntimeBridge) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	for id, ch := range b.pending {
		ch <- runtimeReply{err: errors.New("REMOTE_CONTEXT_LOST")}
		delete(b.pending, id)
	}
	b.mu.Unlock()
	_ = b.input.Close()
	_ = b.session.Close()
	_ = b.client.Close()
}
func (b *sshRuntimeBridge) read(reader io.Reader) {
	defer b.close()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	for scanner.Scan() {
		var message runtimeMessage
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			return
		}
		b.mu.Lock()
		ch := b.pending[message.ID]
		delete(b.pending, message.ID)
		b.mu.Unlock()
		if ch != nil {
			var err error
			if message.Error != "" {
				err = errors.New(message.Error)
			}
			ch <- runtimeReply{payload: message.Payload, err: err}
		}
	}
}
func (b *sshRuntimeBridge) call(ctx context.Context, kind string, payload proto.Message) (proto.Message, error) {
	raw, err := protojson.Marshal(payload)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	ch := make(chan runtimeReply, 1)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	b.pending[id] = ch
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.pending, id); b.mu.Unlock() }()
	request, _ := json.Marshal(runtimeMessage{ID: id, Kind: kind, Payload: raw})
	b.writeMu.Lock()
	_, err = b.input.Write(append(request, '\n'))
	b.writeMu.Unlock()
	if err != nil {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	select {
	case <-ctx.Done():
		return nil, errors.New("REMOTE_RESULT_UNKNOWN")
	case result := <-ch:
		if result.err != nil {
			return nil, result.err
		}
		message, err := connectorprotocol.Result(kind)
		if err != nil {
			return nil, err
		}
		if err = protojson.Unmarshal(result.payload, message); err != nil {
			return nil, errors.New("REMOTE_COMPLETION_INVALID")
		}
		return message, nil
	}
}

// Runs under the SSH-authenticated target account. The process protocol is
// private to the trusted Connector; scripts receive neither stdin nor tickets.
func runRemoteRuntime(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return errors.New("remote-runtime accepts no arguments")
	}
	manager := remoterunner.New("")
	defer manager.Close()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	var writer sync.Mutex
	var tasks sync.WaitGroup
	for scanner.Scan() {
		var request runtimeMessage
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == "" {
			return errors.New("REMOTE_INPUT_INVALID")
		}
		tasks.Add(1)
		go func(request runtimeMessage) {
			defer tasks.Done()
			var result proto.Message
			var err error
			switch request.Kind {
			case "remote_execution_batch":
				var payload connectorv1.RemoteExecutionBatch
				err = protojson.Unmarshal(request.Payload, &payload)
				if err == nil {
					result, err = manager.Execute(ctx, &payload)
				}
			case "remote_execution_control":
				var payload connectorv1.RemoteExecutionControl
				err = protojson.Unmarshal(request.Payload, &payload)
				if err == nil {
					result, err = manager.Control(ctx, &payload)
				}
			case "remote_execution_file":
				var payload connectorv1.RemoteExecutionFile
				err = protojson.Unmarshal(request.Payload, &payload)
				if err == nil {
					result, err = manager.File(ctx, &payload)
				}
			default:
				err = errors.New("REMOTE_INPUT_INVALID")
			}
			response := runtimeMessage{ID: request.ID}
			if err != nil {
				response.Error = "REMOTE_RUNTIME_FAILED"
				if strings.HasPrefix(err.Error(), "REMOTE_") {
					response.Error = err.Error()
				}
			} else {
				response.Payload, _ = protojson.Marshal(result)
			}
			encoded, _ := json.Marshal(response)
			writer.Lock()
			_, _ = os.Stdout.Write(append(encoded, '\n'))
			writer.Unlock()
		}(request)
	}
	manager.Close()
	tasks.Wait()
	return scanner.Err()
}
