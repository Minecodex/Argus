package alerting

import (
	"encoding/json"
	"fmt"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Notification decisions may only update notification metadata. instanceDTO is
// an authorization projection for readers, never the authoritative state to
// write back: it deliberately narrows frozen policy/source participation.
func notificationStateForStorage(row db.AlertInstance, visible InstanceState) ([]byte, error) {
	var stored InstanceState
	if err := json.Unmarshal(row.State, &stored); err != nil {
		return nil, fmt.Errorf("decode stored alert notification state: %w", err)
	}
	stored.NotificationState = visible.NotificationState
	stored.NoiseReasons = visible.NoiseReasons
	stored.LastNotificationAt = visible.LastNotificationAt
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode stored alert notification state: %w", err)
	}
	return raw, nil
}
