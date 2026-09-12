package connector

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type removalBinding struct {
	OperationID uuid.UUID `json:"operation_id"`
	ConnectorID uuid.UUID `json:"connector_id"`
	Generation  int64     `json:"removal_generation"`
}

// Persist authorization before removing identity.json. A later retry with no
// installed identity is allowed only for this previously bound operation.
func ensureLocalRemovalBinding(identityDirectory, journal string, expected removalBinding) error {
	raw, err := os.ReadFile(filepath.Join(identityDirectory, identityFile))
	if err == nil {
		var identity struct {
			ConnectorID string `json:"connector_id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ConnectorID != expected.ConnectorID.String() {
			return errors.New("TARGET_IDENTITY_CHANGED")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	path := filepath.Join(journal, "binding.json")
	stored, bindingErr := os.ReadFile(path)
	if bindingErr == nil {
		var binding removalBinding
		if json.Unmarshal(stored, &binding) != nil || binding != expected {
			return errors.New("TARGET_IDENTITY_CHANGED")
		}
		return nil
	}
	if !errors.Is(bindingErr, os.ErrNotExist) {
		return bindingErr
	}
	if err != nil {
		return errors.New("TARGET_IDENTITY_CHANGED")
	}
	encoded, _ := json.Marshal(expected)
	return atomicPrivateWrite(path, encoded)
}
