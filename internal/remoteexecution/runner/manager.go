// Package runner supervises approved command batches independently of a human
// PTY and of the Connector's current network connection.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
)

const maxOutput = 8 << 20

type commandRuntime interface {
	Run(context.Context, string, string, string) (runtimeResult, error)
	Close() error
	Version() string
}
type runtimeResult struct {
	ExitCode       int
	Directory      string
	Stdout, Stderr []byte
	Truncated      bool
}
type session struct {
	mu                                               sync.Mutex
	id, generation, username, sid, interpreter, root string
	revision                                         uint64
	runtime                                          commandRuntime
	activeRequest                                    string
	running                                          bool
	lost                                             bool
	touched                                          time.Time
	completed                                        map[string]*connectorv1.RemoteExecutionBatchResult
	completedHashes                                  map[string]string
	files                                            *os.Root
	exchanges                                        map[string]*exchangeRoots
}
type exchangeRoots struct{ input, output *os.Root }
type Manager struct {
	mu            sync.Mutex
	sessions      map[string]*session
	BaseDirectory string
	IdleTimeout   time.Duration
	// Set only by the trusted Kubernetes helper launcher, never by tool input.
	KubernetesResourceID  string
	KubernetesInterpreter string
	KubernetesPodUID      string
}

func New(base string) *Manager {
	if base == "" {
		base = filepath.Join(os.TempDir(), "argus")
	}
	return &Manager{sessions: map[string]*session{}, BaseDirectory: base, IdleTimeout: 30 * time.Minute}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.mu.Lock()
		s.lost = true
		if s.runtime != nil {
			_ = s.runtime.Close()
		}
		s.closeRoots()
		s.mu.Unlock()
	}
}
func validID(v string) bool { _, err := uuid.Parse(v); return err == nil }
func (m *Manager) validateBinding(b *connectorv1.RemoteExecutionBatch) error {
	if b == nil || !validID(b.SessionId) || !validID(b.SessionGeneration) || !validID(b.RequestId) || !validID(b.IdentityBinding) || !validID(b.ResourceId) || b.ActionRef == "" || len(b.ApprovedContentHash) != 64 || b.Username == "" {
		return errors.New("REMOTE_BINDING_INVALID")
	}
	if b.ResourceType == "kubernetes_cluster" && m.KubernetesResourceID != "" {
		if b.ResourceId != m.KubernetesResourceID || b.Interpreter != m.KubernetesInterpreter || b.PodUid != m.KubernetesPodUID {
			return errors.New("REMOTE_BINDING_INVALID")
		}
		if goruntime.GOOS != "linux" || (b.Interpreter != "kubectl" && b.Interpreter != "pod_bash") {
			return errors.New("REMOTE_PLATFORM_UNAVAILABLE")
		}
		return nil
	}
	if b.ResourceType != "host" || (b.Interpreter != "bash" && b.Interpreter != "powershell") {
		return errors.New("REMOTE_PLATFORM_UNAVAILABLE")
	}
	if goruntime.GOOS == "windows" && b.Interpreter != "powershell" || goruntime.GOOS != "windows" && b.Interpreter != "bash" {
		return errors.New("REMOTE_INTERPRETER_UNAVAILABLE")
	}
	current, err := user.Current()
	if err != nil {
		return err
	}
	if goruntime.GOOS != "windows" && !strings.EqualFold(current.Username, b.Username) && current.Uid != b.Username {
		return errors.New("REMOTE_IDENTITY_MISMATCH")
	}
	if goruntime.GOOS == "windows" && (b.IdentitySid == "" || !strings.EqualFold(current.Uid, b.IdentitySid)) {
		return errors.New("REMOTE_IDENTITY_MISMATCH")
	}
	return nil
}
func (m *Manager) Open(ctx context.Context, b *connectorv1.RemoteExecutionBatch) (*connectorv1.RemoteExecutionControlResult, error) {
	if err := m.validateBinding(b); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[b.SessionId+"/"+b.SessionGeneration]
	if s != nil {
		if s.generation != b.SessionGeneration || s.username != b.Username || s.sid != b.IdentitySid || s.interpreter != b.Interpreter {
			return nil, errors.New("REMOTE_CONTEXT_LOST")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.lost {
			return nil, errors.New("REMOTE_CONTEXT_LOST")
		}
	} else {
		root := filepath.Join(m.BaseDirectory, b.SessionId, b.SessionGeneration)
		if _, err := os.Lstat(root); err == nil {
			return nil, errors.New("REMOTE_CONTEXT_LOST")
		}
		if err := os.MkdirAll(m.BaseDirectory, 0700); err != nil {
			return nil, err
		}
		base, err := os.OpenRoot(m.BaseDirectory)
		if err != nil {
			return nil, err
		}
		defer base.Close()
		if err = rootDirectory(base, b.SessionId); err != nil {
			return nil, err
		}
		if err = base.Mkdir(filepath.Join(b.SessionId, b.SessionGeneration), 0700); err != nil {
			return nil, err
		}
		files, err := base.OpenRoot(filepath.Join(b.SessionId, b.SessionGeneration))
		if err != nil {
			return nil, err
		}
		owner, _ := json.Marshal(map[string]string{"session_id": b.SessionId, "generation": b.SessionGeneration, "username": b.Username, "identity_sid": b.IdentitySid})
		if err := files.WriteFile(".argus-owner.json", owner, 0600); err != nil {
			files.Close()
			return nil, err
		}
		interpreter := b.Interpreter
		if interpreter == "kubectl" || interpreter == "pod_bash" {
			interpreter = "bash"
		}
		runtime, err := newRuntime(ctx, root, interpreter, m.KubernetesResourceID != "")
		if err != nil {
			files.Close()
			return nil, err
		}
		s = &session{id: b.SessionId, generation: b.SessionGeneration, username: b.Username, sid: b.IdentitySid, interpreter: b.Interpreter, root: root, runtime: runtime, touched: time.Now(), completed: map[string]*connectorv1.RemoteExecutionBatchResult{}, completedHashes: map[string]string{}, files: files, exchanges: map[string]*exchangeRoots{}}
		m.sessions[b.SessionId+"/"+b.SessionGeneration] = s
	}
	if s.running && s.activeRequest != b.RequestId {
		return nil, errors.New("REMOTE_SESSION_BUSY")
	}
	input, output, err := s.directories(b.RequestId)
	if err != nil {
		return nil, err
	}
	s.touched = time.Now()
	account := ""
	if current, e := user.Current(); e == nil {
		account = current.Username + " (" + current.Uid + ")"
	}
	return &connectorv1.RemoteExecutionControlResult{SessionId: s.id, SessionGeneration: s.generation, Status: "active", ContextRevision: s.revision, Username: s.username, IdentitySid: s.sid, InterpreterVersion: s.runtime.Version(), InputDirectory: input, OutputDirectory: output, RuntimeAccount: account}, nil
}
func (s *session) directories(request string) (string, string, error) {
	if !validID(request) {
		return "", "", errors.New("REMOTE_BINDING_INVALID")
	}
	root := filepath.Join(s.root, request)
	if s.exchanges[request] == nil {
		for _, p := range []string{request, request + "/input", request + "/output"} {
			if err := rootDirectory(s.files, p); err != nil {
				return "", "", err
			}
		}
		input, err := s.files.OpenRoot(request + "/input")
		if err != nil {
			return "", "", err
		}
		output, err := s.files.OpenRoot(request + "/output")
		if err != nil {
			input.Close()
			return "", "", err
		}
		s.exchanges[request] = &exchangeRoots{input: input, output: output}
	}
	return filepath.Join(root, "input"), filepath.Join(root, "output"), nil
}
func (s *session) closeRoots() {
	for _, r := range s.exchanges {
		r.input.Close()
		r.output.Close()
	}
	if s.files != nil {
		s.files.Close()
	}
}
func (m *Manager) lookup(id, generation string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id+"/"+generation]
	if s == nil || s.generation != generation {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	return s, nil
}
func (m *Manager) Control(ctx context.Context, c *connectorv1.RemoteExecutionControl) (*connectorv1.RemoteExecutionControlResult, error) {
	if c.Operation == "reclaim" {
		return m.reclaim(c)
	}
	if c.Operation == "open" {
		return m.Open(ctx, c.Binding)
	}
	if c.Operation == "recover_files" {
		return m.RecoverFiles(ctx, c.Binding)
	}
	s, err := m.lookup(c.SessionId, c.SessionGeneration)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Operation == "stop" || c.Operation == "close" {
		if s.runtime == nil {
			return &connectorv1.RemoteExecutionControlResult{SessionId: s.id, SessionGeneration: s.generation, Status: "stop_unknown"}, errors.New("REMOTE_STOP_UNKNOWN")
		}
		if c.Operation == "close" && s.running {
			return nil, errors.New("REMOTE_SESSION_BUSY")
		}
		if s.activeRequest != "" && c.RequestId != s.activeRequest {
			return nil, errors.New("REMOTE_BINDING_INVALID")
		}
		err = s.runtime.Close()
		s.lost = true
		status := "stopped"
		if err != nil {
			status = "stop_unknown"
		}
		return &connectorv1.RemoteExecutionControlResult{SessionId: s.id, SessionGeneration: s.generation, Status: status, ContextRevision: s.revision}, err
	}
	if c.Operation != "status" {
		return nil, errors.New("REMOTE_INPUT_INVALID")
	}
	status := "active"
	if s.lost {
		status = "lost"
	}
	version := ""
	if s.runtime != nil {
		version = s.runtime.Version()
	}
	return &connectorv1.RemoteExecutionControlResult{SessionId: s.id, SessionGeneration: s.generation, Status: status, ContextRevision: s.revision, Username: s.username, IdentitySid: s.sid, InterpreterVersion: version}, nil
}
func (m *Manager) Execute(ctx context.Context, b *connectorv1.RemoteExecutionBatch) (*connectorv1.RemoteExecutionBatchResult, error) {
	if err := m.validateBinding(b); err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(b.Script))
	if hex.EncodeToString(hash[:]) != b.ScriptSha256 || b.TimeoutSeconds < 1 || b.TimeoutSeconds > 1800 || b.MaxOutputBytes > maxOutput || b.Script == "" {
		return nil, errors.New("REMOTE_INPUT_INVALID")
	}
	s, err := m.lookup(b.SessionId, b.SessionGeneration)
	if err != nil {
		return nil, err
	}
	key := b.RequestId + "/" + stringID(b.Sequence)
	s.mu.Lock()
	if old := s.completed[key]; old != nil {
		if s.completedHashes[key] != b.ScriptSha256 {
			s.mu.Unlock()
			return nil, errors.New("REMOTE_COMMAND_CONFLICT")
		}
		s.mu.Unlock()
		return proto.Clone(old).(*connectorv1.RemoteExecutionBatchResult), nil
	}
	if s.lost || s.revision != b.ContextRevision {
		s.mu.Unlock()
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	if s.running {
		s.mu.Unlock()
		return nil, errors.New("REMOTE_SESSION_BUSY")
	}
	input, output, err := s.directories(b.RequestId)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.running = true
	s.activeRequest = b.RequestId
	s.touched = time.Now()
	exchange := s.exchanges[b.RequestId]
	s.mu.Unlock()
	executeCtx, cancel := context.WithTimeout(ctx, time.Duration(b.TimeoutSeconds)*time.Second)
	defer cancel()
	capture, runErr := s.runtime.Run(executeCtx, b.Script, input, output)
	result := &connectorv1.RemoteExecutionBatchResult{RequestId: b.RequestId, SessionId: s.id, SessionGeneration: s.generation, ContextRevision: b.ContextRevision + 1, Sequence: b.Sequence, ExitCode: int32(capture.ExitCode), WorkingDirectory: capture.Directory, Truncated: capture.Truncated}
	result.Stdout = capture.Stdout[:min(len(capture.Stdout), int(b.MaxOutputBytes))]
	result.Stderr = capture.Stderr[:min(len(capture.Stderr), int(b.MaxOutputBytes)-len(result.Stdout))]
	result.Truncated = result.Truncated || len(result.Stdout) != len(capture.Stdout) || len(result.Stderr) != len(capture.Stderr)
	// Full bounded output travels through the chunked file channel. The command
	// receipt remains small enough for the control stream and model projection.
	if len(result.Stdout) > 0 || len(result.Stderr) > 0 {
		root := exchange.output
		if e := rootDirectory(root, ".argus-logs"); e != nil {
			runErr = e
		} else {
			for _, log := range []struct {
				name string
				data []byte
			}{{"stdout", result.Stdout}, {"stderr", result.Stderr}} {
				if len(log.data) == 0 {
					continue
				}
				name := ".argus-logs/" + stringID(b.Sequence) + "." + log.name + ".txt"
				e := rootFile(root, name)
				if e != nil {
					runErr = e
					break
				}
				f, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if e != nil {
					runErr = e
					break
				}
				_, e = f.Write(log.data)
				closeErr := f.Close()
				if e != nil {
					runErr = e
					break
				}
				if closeErr != nil {
					runErr = closeErr
					break
				}
			}
		}
	}
	if len(result.Stdout) > 64<<10 {
		result.Stdout = result.Stdout[:64<<10]
		result.Truncated = true
	}
	if len(result.Stderr) > 32<<10 {
		result.Stderr = result.Stderr[:32<<10]
		result.Truncated = true
	}
	if runErr != nil {
		result.Terminated = s.runtime.Close() == nil
		result.SessionLost = true
		result.ErrorCode = "REMOTE_CONTEXT_LOST"
		if executeCtx.Err() != nil {
			result.ErrorCode = "REMOTE_EXECUTION_TIMEOUT"
		}
	} else {
		result.Terminated = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lost {
		result.SessionLost = true
		if result.ErrorCode == "" {
			result.ErrorCode = "REMOTE_CONTEXT_LOST"
		}
	}
	s.revision++
	s.running = false
	s.activeRequest = ""
	s.lost = result.SessionLost
	s.touched = time.Now()
	s.completed[key] = proto.Clone(result).(*connectorv1.RemoteExecutionBatchResult)
	s.completedHashes[key] = b.ScriptSha256
	return result, nil
}
func stringID(n uint32) string {
	const chars = "0123456789"
	if n == 0 {
		return "0"
	}
	out := []byte{}
	for n > 0 {
		out = append([]byte{chars[n%10]}, out...)
		n /= 10
	}
	return string(out)
}
