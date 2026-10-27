package argusdev

import (
	"github.com/kakj-go/Argus/internal/monitoring"
	"testing"
	"time"
)

func TestActiveTargetAuthorizationDoesNotFreezeAtItsFirstIngestSample(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	first := now.Add(-4 * time.Minute)
	row := monitoring.Collection{DesiredEnabled: true, LastSampleAt: &first}
	path, q := plan8TargetAuthorizationQuery(row, "postgresql_backends", now)
	if path != "/enterprise/metrics/query" || q["step_seconds"] != nil {
		t.Fatal("active target used historical range protocol")
	}
	rangeValue := q["time_range"].(map[string]any)
	end, err := time.Parse(time.RFC3339Nano, rangeValue["to"].(string))
	if err != nil || end.Before(now) {
		t.Fatal("active query excluded samples that arrived after first Ingest")
	}
	row.DesiredEnabled = false
	path, q = plan8TargetAuthorizationQuery(row, "postgresql_backends", now)
	if path != "/enterprise/metrics/query_range" || q["step_seconds"] != float64(15) {
		t.Fatal("stopped target lost its actual historical range")
	}
	rangeValue = q["time_range"].(map[string]any)
	end, err = time.Parse(time.RFC3339Nano, rangeValue["to"].(string))
	if err != nil || !end.Equal(first.Add(time.Second)) {
		t.Fatal("stopped target queried an unobserved current time")
	}
}
