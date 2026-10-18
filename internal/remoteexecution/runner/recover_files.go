package runner

import (
	"context"
	"encoding/json"
	"errors"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"os"
	"path/filepath"
	"time"
)

// RecoverFiles attaches only owned directories. It never starts an interpreter
// and cannot make a lost generation executable again.
func (m *Manager) RecoverFiles(ctx context.Context, b *connectorv1.RemoteExecutionBatch) (*connectorv1.RemoteExecutionControlResult, error) {
	if err := m.validateBinding(b); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := b.SessionId + "/" + b.SessionGeneration
	if existing := m.sessions[key]; existing != nil {
		existing.mu.Lock()
		defer existing.mu.Unlock()
		if existing.running {
			return nil, errors.New("REMOTE_SESSION_BUSY")
		}
		if err := existing.recoverExchange(b.RequestId); err != nil {
			return nil, err
		}
		return &connectorv1.RemoteExecutionControlResult{SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, Status: "files_only"}, nil
	}
	base, err := os.OpenRoot(m.BaseDirectory)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	root, err := base.OpenRoot(filepath.Join(b.SessionId, b.SessionGeneration))
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			root.Close()
		}
	}()
	raw, err := root.ReadFile(".argus-owner.json")
	if err != nil {
		return nil, err
	}
	var owner map[string]string
	if json.Unmarshal(raw, &owner) != nil || owner["session_id"] != b.SessionId || owner["generation"] != b.SessionGeneration || owner["username"] != b.Username || owner["identity_sid"] != b.IdentitySid {
		return nil, errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
	}
	s := &session{id: b.SessionId, generation: b.SessionGeneration, username: b.Username, sid: b.IdentitySid, interpreter: b.Interpreter, root: filepath.Join(m.BaseDirectory, b.SessionId, b.SessionGeneration), files: root, exchanges: map[string]*exchangeRoots{}, lost: true, touched: time.Now(), completed: map[string]*connectorv1.RemoteExecutionBatchResult{}, completedHashes: map[string]string{}}
	if err := s.recoverExchange(b.RequestId); err != nil {
		return nil, err
	}
	m.sessions[key] = s
	keep = true
	return &connectorv1.RemoteExecutionControlResult{SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, Status: "files_only"}, nil
}

func (s *session) recoverExchange(request string) error {
	if s.exchanges[request] != nil {
		return nil
	}
	// Recovery opens existing paths only. It must not create directories or
	// follow links installed after the original interpreter disappeared.
	for _, name := range []string{request, request + "/input", request + "/output"} {
		info, err := s.files.Lstat(name)
		if err != nil {
			return err
		}
		if !info.IsDir() || isLinkOrReparse(filepath.Join(s.files.Name(), name), info) {
			return errors.New("REMOTE_PATH_INVALID")
		}
	}
	input, err := s.files.OpenRoot(request + "/input")
	if err != nil {
		return err
	}
	output, err := s.files.OpenRoot(request + "/output")
	if err != nil {
		input.Close()
		return err
	}
	s.exchanges[request] = &exchangeRoots{input: input, output: output}
	return nil
}
