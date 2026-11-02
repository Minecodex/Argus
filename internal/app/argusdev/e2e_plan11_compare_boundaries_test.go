package argusdev

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

func TestComparisonChecksAllAvailableBoundariesBeforeCompletion(t *testing.T) {
	host, board := uuid.New(), uuid.New()
	h := &plan6HostLoop{HostID: host.String(), DashboardID: board.String()}
	window := plan11ComparisonWindow{From: time.Unix(100, 0).UTC(), To: time.Unix(400, 0).UTC()}
	job := func() dashboard.QueryJobView {
		return dashboard.QueryJobView{DashboardID: board, Status: "complete", Manifest: &dashboard.QueryManifest{Execution: dashboard.Execution{From: window.From, To: window.To, Resources: []dashboard.ResourceScope{{Type: "host", ID: host}}}}}
	}
	if err := plan11ComparisonReadBoundaries(h, window, "dashboard", nil, nil, []dashboard.QueryJobView{job()}); err != nil {
		t.Fatal("valid single-Host evidence rejected", err)
	}
	// r29 had a successful Run but no published report. Its available manifest
	// still contained four Hosts, so completion/accounting cannot make it safe.
	fourHosts := job()
	for range 3 {
		fourHosts.Manifest.Execution.Resources = append(fourHosts.Manifest.Execution.Resources, dashboard.ResourceScope{Type: "host", ID: uuid.New()})
	}
	missing := dashboard.QueryJobView{DashboardID: board, Status: "failed"}
	narrowed := job()
	narrowed.Manifest.Execution.To = window.From.Add(time.Minute)
	for _, prior := range []dashboard.QueryJobView{missing, narrowed} {
		if err := plan11ComparisonReadBoundaries(h, window, "dashboard", nil, nil, []dashboard.QueryJobView{prior, fourHosts}); !errors.Is(err, errPlan11ComparisonBoundary) {
			t.Fatal("an earlier ordinary failure hid the later four-Host execution", err)
		}
	}
	wrongType := job()
	wrongType.Manifest.Execution.Resources[0].Type = "monitoring_target"
	if err := plan11ComparisonReadBoundaries(h, window, "dashboard", nil, nil, []dashboard.QueryJobView{wrongType}); !errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("another resource type with the same UUID passed", err)
	}
	if err := plan11ComparisonReadBoundaries(h, window, "dashboard", nil, nil, []dashboard.QueryJobView{narrowed}); err == nil || errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("in-bounds shortening must remain an ordinary failed task", err)
	}
}

func TestComparisonDoesNotHideLaterMetricOrOriginalFileBoundary(t *testing.T) {
	host := uuid.NewString()
	h := &plan6HostLoop{HostID: host}
	window := plan11ComparisonWindow{From: time.Unix(100, 0).UTC(), To: time.Unix(400, 0).UTC()}
	metric := func(id string, to time.Time) plan8ModelToolFact {
		call := plan8ModelToolFact{Tool: "tool.invoke", Status: "succeeded"}
		call.Input.Category, call.Input.Name = "metric", "query"
		call.Input.Arguments = map[string]any{"resource_ids": []any{id}, "from": window.From.Format(time.RFC3339), "to": to.Format(time.RFC3339)}
		return call
	}
	calls := []plan8ModelToolFact{metric(host, window.From.Add(time.Minute)), metric(uuid.NewString(), window.To)}
	if err := plan11ComparisonReadBoundaries(h, window, "metric", calls, nil, nil); !errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("first narrowed query hid a later foreign Host", err)
	}
	files := []plan11QueryFile{{Body: []byte(`{"data":{"result":[{"metric":{"argus_resource_id":"another-host"}}]}}`)}}
	if err := plan11ComparisonReadBoundaries(h, window, "metric", nil, files, nil); !errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("original foreign data was hidden by missing conclusion/report metadata", err)
	}
}
