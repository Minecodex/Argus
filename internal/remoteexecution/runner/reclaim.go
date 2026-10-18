package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func (m *Manager) reclaim(c *connectorv1.RemoteExecutionControl) (*connectorv1.RemoteExecutionControlResult, error) {
	if err := m.validateBinding(c.Binding); err != nil {
		return nil, err
	}
	if c.SessionId != c.Binding.SessionId || c.SessionGeneration != c.Binding.SessionGeneration {
		return nil, errors.New("REMOTE_BINDING_INVALID")
	}
	key := c.SessionId + "/" + c.SessionGeneration
	m.mu.Lock()
	s := m.sessions[key]
	m.mu.Unlock()
	if s != nil {
		s.mu.Lock()
		if s.running {
			s.mu.Unlock()
			return nil, errors.New("REMOTE_SESSION_BUSY")
		}
		s.lost = true
		var err error
		if s.runtime != nil {
			err = s.runtime.Close()
		}
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.closeRoots()
		s.mu.Unlock()
	}
	root, err := os.OpenRoot(m.BaseDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return &connectorv1.RemoteExecutionControlResult{SessionId: c.SessionId, SessionGeneration: c.SessionGeneration, Status: "reclaimed"}, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	relative := filepath.Join(c.SessionId, c.SessionGeneration)
	parent, err := root.OpenRoot(relative)
	if errors.Is(err, os.ErrNotExist) {
		return &connectorv1.RemoteExecutionControlResult{SessionId: c.SessionId, SessionGeneration: c.SessionGeneration, Status: "reclaimed"}, nil
	}
	if err != nil {
		return nil, err
	}
	raw, err := parent.ReadFile(".argus-owner.json")
	parent.Close()
	if err != nil {
		return nil, errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
	}
	var owner map[string]string
	if json.Unmarshal(raw, &owner) != nil || owner["session_id"] != c.SessionId || owner["generation"] != c.SessionGeneration || owner["username"] != c.Binding.Username || owner["identity_sid"] != c.Binding.IdentitySid {
		return nil, errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
	}
	if err = root.RemoveAll(relative); err != nil {
		return nil, err
	}
	m.mu.Lock()
	delete(m.sessions, key)
	m.mu.Unlock()
	return &connectorv1.RemoteExecutionControlResult{SessionId: c.SessionId, SessionGeneration: c.SessionGeneration, Status: "reclaimed"}, nil
}
