package alerting

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestNoiseMetadataWritePreservesAuthoritativeFactsAndTimers(t *testing.T) {
	a, b, sa, sb := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	policy, _ := json.Marshal(PolicySpec{Resources: ResourceSelector{Mode: "explicit", IDs: []uuid.UUID{a, b}, Types: []string{"host"}}})
	sources, _ := json.Marshal([]db.AlertMetricSourcesRow{{ID: sa, ResourceID: a}, {ID: sb, ResourceID: b}})
	at := time.Now().UTC().Truncate(time.Second)
	value := 95.0
	stored := InstanceState{PolicySnapshot: policy, SourceSnapshot: sources, LatestSourceSnapshot: sources, PendingSince: &at, LastSlot: at, Value: &value, NotificationState: "pending"}
	raw, _ := json.Marshal(stored)
	row := db.AlertInstance{State: raw, ResourceIds: []uuid.UUID{a}, SourceIds: []uuid.UUID{sa}}
	visible, err := instanceDTO(row)
	if err != nil {
		t.Fatal(err)
	}
	visible.State.NotificationState = "muted"
	visible.State.NoiseReasons = []string{"owned silence"}
	visible.State.LastNotificationAt = &at
	// A presentation-layer value/timer change must not rewrite evaluated data.
	other := 0.0
	visible.State.Value = &other
	visible.State.PendingSince = nil
	updated, err := notificationStateForStorage(row, visible.State)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	_ = json.Unmarshal(raw, &before)
	_ = json.Unmarshal(updated, &after)
	for _, key := range []string{"notification_state", "noise_reasons", "last_notification_at"} {
		delete(before, key)
		delete(after, key)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("notification write rewrote facts, provenance or continuous-window state")
	}
	if !json.Valid(updated) || !reflect.DeepEqual(row.State, raw) {
		t.Fatal("invalid output or source mutation")
	}
}
