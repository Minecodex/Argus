package argusdev

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

func plan8ScopedLogMatches(value map[string]any, resource uuid.UUID, marker string) error {
	if err := assertKQLResponse(value, "", 1); err != nil {
		return err
	}
	meta, _ := value["meta"].(map[string]any)
	if meta["partial"] != false {
		return errors.New("scoped log response was incomplete")
	}
	rows := value["data"].([]any)
	row, ok := rows[0].(map[string]any)
	if !ok || row["resource_id"] != resource.String() {
		return errors.New("log query escaped its observation owner")
	}
	body, ok := row["body"].(string)
	if !ok {
		return errors.New("log query returned no preserved body")
	}
	var object map[string]any
	if json.Unmarshal([]byte(body), &object) == nil && object != nil {
		// Structured file/Journald bodies retain their metadata. The marker is
		// an actual message value, not the entire serialized JSON document.
		for _, key := range []string{"message", "MESSAGE"} {
			if message, ok := object[key].(string); ok && strings.Contains(message, marker) {
				return nil
			}
		}
		return errors.New("structured log body did not contain the expected message")
	}
	if strings.Contains(body, marker) {
		return nil
	}
	return errors.New("text log body did not contain the expected message")
}
