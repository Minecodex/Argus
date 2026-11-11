package alerting

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresMissingEpisodeClosesWhenParticipatingSourceScopeChanges(t *testing.T) {
	f := newAlertFixture(t)
	a := db.AlertMetricSourcesRow{ID: uuid.New(), Generation: uuid.New(), ResourceID: f.host}
	b := db.AlertMetricSourcesRow{ID: uuid.New(), Generation: uuid.New(), ResourceID: f.host}
	replaced := a
	replaced.Generation = uuid.New()
	tests := []struct {
		name       string
		before     []db.AlertMetricSourcesRow
		after      []db.AlertMetricSourcesRow
		explicit   []uuid.UUID
		wantReason string
	}{
		{"added aggregate source", []db.AlertMetricSourcesRow{a}, []db.AlertMetricSourcesRow{a, b}, nil, "source_changed"},
		{"removed aggregate source", []db.AlertMetricSourcesRow{a, b}, []db.AlertMetricSourcesRow{a}, nil, "source_changed"},
		{"same scope reordered", []db.AlertMetricSourcesRow{a, b}, []db.AlertMetricSourcesRow{b, a}, nil, ""},
		{"new installation generation", []db.AlertMetricSourcesRow{a}, []db.AlertMetricSourcesRow{replaced}, nil, "source_changed"},
		{"explicit source ignores unrelated addition", []db.AlertMetricSourcesRow{a}, []db.AlertMetricSourcesRow{a, b}, []uuid.UUID{a.ID}, ""},
		{"explicit source ignores unrelated removal", []db.AlertMetricSourcesRow{a, b}, []db.AlertMetricSourcesRow{a}, []uuid.UUID{a.ID}, ""},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			snapshot, _ := json.Marshal(item.before)
			state, _ := json.Marshal(InstanceState{Kind: "condition", SourceSnapshot: snapshot})
			row := db.AlertInstance{EnterpriseID: f.a.EnterpriseID, ResourceIds: []uuid.UUID{f.host}, SourceIds: item.explicit, State: state, Lifecycle: "firing"}
			result := Evaluation{SourcesCaptured: true, Sources: item.after, ResourceIDs: []uuid.UUID{f.host}, Evaluator: Actor{SubjectID: f.sa}}
			got, err := missingEpisodeClosure(t.Context(), f.s.Store.Queries, row, result)
			if err != nil || got != item.wantReason {
				t.Fatalf("closure = %q, error %v, want %q", got, err, item.wantReason)
			}
			result.SourcesCaptured = false
			got, err = missingEpisodeClosure(t.Context(), f.s.Store.Queries, row, result)
			if err != nil || got != "" {
				t.Fatalf("uncaptured query fabricated a closure: %q, %v", got, err)
			}
		})
	}
}

func TestPostgresCurrentAlertsExcludeUntriggeredNormalQueryState(t *testing.T) {
	f := newAlertFixture(t)
	policy := f.createPolicy()
	at := time.Now().UTC()
	tests := []struct {
		lifecycle, health, kind string
		triggered               bool
	}{
		{"normal", "no_data", "condition", false},
		{"normal", "normal", "condition", false},
		{"pending", "normal", "condition", false},
		{"firing", "no_data", "condition", true},
		{"firing", "error", "health", true},
	}
	for _, item := range tests {
		state, _ := json.Marshal(InstanceState{Name: "Scoped state", Kind: item.kind, PolicySnapshot: policy.Spec, Labels: map[string]string{}})
		started := timestamp(at)
		if !item.triggered {
			started.Valid = false
		}
		_, err := f.s.Store.Queries.CreateAlertInstance(t.Context(), db.CreateAlertInstanceParams{
			ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, PolicyID: policy.ID, PolicyVersion: policy.Version,
			Generation: policy.Generation, Fingerprint: uuid.NewString(), ResourceIds: []uuid.UUID{f.host},
			SourceIds: []uuid.UUID{}, State: state, Lifecycle: item.lifecycle, Health: item.health,
			Severity: "warning", StartedAt: started, LastEvaluatedAt: timestamp(at),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.s.Instances(t.Context(), f.a, ListFilter{Limit: 1}, false)
	if err != nil || page.Total != 3 || len(page.Items) != 1 {
		t.Fatalf("current-page count includes internal normal state: %#v, %v", page, err)
	}
	for {
		for _, row := range page.Items {
			if row.Lifecycle == "normal" {
				t.Fatal("never-triggered state appeared as a current alert")
			}
		}
		if page.NextCursor == nil {
			break
		}
		page, err = f.s.Instances(t.Context(), f.a, ListFilter{Limit: 1, Cursor: *page.NextCursor}, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	summary, err := f.s.Summary(t.Context(), f.a)
	if err != nil || summary.CurrentTotal != 3 || summary.Active != 2 || summary.Unacknowledged != 3 {
		t.Fatalf("summary disagrees with actionable current alerts: %#v, %v", summary, err)
	}
}
