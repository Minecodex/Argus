package alerting

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"testing"
	"time"
)

func TestContinuousTriggerFailureAndRecoveryHysteresis(t *testing.T) {
	p := PolicySpec{IntervalSeconds: 30, ForSeconds: 60, RecoverySeconds: 60}
	r := Rule{ID: uuid.New(), Expression: "cpu_usage", Operator: ">", Threshold: 90, RecoveryThreshold: 75, Severity: "critical"}
	o := Observation{Rule: r, RuleID: r.ID, Value: 95, Firing: true}
	at := time.Unix(1800000000, 0).UTC()
	life := "normal"
	state := InstanceState{}
	life, state, event := advance(life, state, o, p, at)
	if life != "pending" || event != "" {
		t.Fatal(life, event)
	}
	life, state, _ = advance(life, state, o, p, at.Add(30*time.Second))
	state = evaluationFailure(state)
	life, state, event = advance(life, state, o, p, at.Add(60*time.Second))
	if life != "pending" || event != "" {
		t.Fatal("failed evaluation continued trigger timer")
	}
	life, state, _ = advance(life, state, o, p, at.Add(90*time.Second))
	life, state, event = advance(life, state, o, p, at.Add(120*time.Second))
	if life != "firing" || event != "first" {
		t.Fatal(life, event)
	}
	o.Firing = false
	o.Value = 80
	o.Recovering = false
	life, state, event = advance(life, state, o, p, at.Add(150*time.Second))
	if life != "firing" || event != "" {
		t.Fatal("hysteresis fabricated recovery")
	}
	o.Value = 70
	o.Recovering = true
	life, state, _ = advance(life, state, o, p, at.Add(180*time.Second))
	state = evaluationFailure(state)
	life, state, event = advance(life, state, o, p, at.Add(210*time.Second))
	if life != "recovering" || event != "" {
		t.Fatal("failed evaluation continued recovery timer")
	}
	life, state, _ = advance(life, state, o, p, at.Add(240*time.Second))
	life, _, event = advance(life, state, o, p, at.Add(270*time.Second))
	if life != "resolved" || event != "recovery" {
		t.Fatal(life, event)
	}
}

func TestObservationsDecodeActualPrometheusValuesWithoutAnHTTPEnvelope(t *testing.T) {
	resource, source := uuid.New(), uuid.New()
	rule := Rule{ID: uuid.New(), Expression: "cpu", Operator: ">", Threshold: 85, RecoveryThreshold: 75, Severity: "critical"}
	data := promql.Vector{{Metric: labels.FromMap(map[string]string{"argus_resource_id": resource.String(), "argus_source_id": source.String()}), T: 1, F: 95}}
	values, err := decodeObservations(queryengine.Result{Data: data, ResultType: "vector"}, rule, []string{"argus_resource_id", "argus_source_id"}, []uuid.UUID{resource})
	if err != nil || len(values) != 1 || values[0].Value != 95 || !values[0].Firing {
		t.Fatal(values, err)
	}
	scalar, err := decodeObservations(queryengine.Result{Data: promql.Scalar{T: 1, V: 95}, ResultType: "scalar"}, rule, nil, []uuid.UUID{resource})
	if err != nil || len(scalar) != 1 || scalar[0].Value != 95 {
		t.Fatal(scalar, err)
	}
}
func TestCompositeDoesNotJoinDuplicateOrMismatchedMembers(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	p := PolicySpec{Mode: "every", Rules: []Rule{{ID: a}, {ID: b}}}
	o := func(id uuid.UUID, key string, firing, recovery bool) Observation {
		return Observation{RuleID: id, Key: key, Firing: firing, Recovering: recovery, Severity: "warning"}
	}
	if got := combineObservations(p, [][]Observation{{o(a, "r1", true, false), o(a, "r1", true, false)}, {}}); len(got) != 0 {
		t.Fatal("duplicate first rule substituted for missing second rule")
	}
	if got := combineObservations(p, [][]Observation{{o(a, "r1", true, false)}, {o(b, "r2", true, false)}}); len(got) != 0 {
		t.Fatal("cross-resource join")
	}
	got := combineObservations(p, [][]Observation{{o(a, "r1", true, false)}, {o(b, "r1", false, true)}})
	if len(got) != 1 || got[0].Firing || !got[0].Recovering {
		t.Fatal(got)
	}
	p.Mode = "any"
	got = combineObservations(p, [][]Observation{{o(a, "r1", true, false)}, {o(b, "r1", false, true)}})
	if !got[0].Firing || got[0].Recovering {
		t.Fatal(got)
	}
}
func TestObservationRejectsUntrustedResourceAndNonfiniteValue(t *testing.T) {
	id := uuid.New()
	other := uuid.New()
	rule := Rule{ID: uuid.New(), Operator: ">", Threshold: 90, RecoveryThreshold: 70}
	for _, tc := range []struct {
		id, value string
		valid     bool
	}{{id.String(), "95", true}, {other.String(), "95", false}, {id.String(), "NaN", false}, {id.String(), "+Inf", false}} {
		data := map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"argus_resource_id": tc.id}, "value": []any{1800000000, tc.value}}}}
		got, err := decodeObservations(queryengine.Result{Data: data}, rule, []string{"argus_resource_id"}, []uuid.UUID{id})
		if (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
		if tc.valid && (len(got) != 1 || !got[0].Firing) {
			t.Fatal(got)
		}
	}
}
func TestSilenceKeepsEpisodeAndInhibitionRequiresSameResource(t *testing.T) {
	id, other := uuid.New(), uuid.New()
	at := time.Now().UTC()
	target := Instance{ID: uuid.New(), PolicyID: uuid.New(), ResourceIDs: []uuid.UUID{id}, Severity: "warning", Lifecycle: "firing", State: InstanceState{Metric: "cpu_usage"}}
	source := Instance{ID: uuid.New(), ResourceIDs: []uuid.UUID{other}, Severity: "critical", Lifecycle: "firing"}
	raw, _ := json.Marshal(InhibitionSpec{Enabled: true, Source: []Matcher{{Key: "severity", Operator: "=", Value: "critical"}}, Target: []Matcher{{Key: "severity", Operator: "=", Value: "warning"}}, Equal: []string{"resource_id"}, Reason: "parent"})
	state, _ := noiseDecision(target, []Instance{source}, nil, []Configuration{{ID: uuid.New(), Spec: raw}}, at)
	if state != "pending" {
		t.Fatal("cross-resource suppression")
	}
	source.ResourceIDs = []uuid.UUID{id}
	state, reasons := noiseDecision(target, []Instance{source}, nil, []Configuration{{ID: uuid.New(), Spec: raw}}, at)
	if state != "inhibited" || len(reasons) != 1 {
		t.Fatal(state, reasons)
	}
	raw, _ = json.Marshal(SilenceSpec{Enabled: true, ResourceIDs: []uuid.UUID{id}, Metric: "cpu_usage", StartsAt: at.Add(-time.Minute), EndsAt: at.Add(time.Minute), Reason: "deployment"})
	state, reasons = noiseDecision(target, nil, []Configuration{{ID: uuid.New(), Spec: raw}}, nil, at)
	if state != "muted" || target.Lifecycle != "firing" || len(reasons) != 1 {
		t.Fatal(state)
	}
	state, _ = noiseDecision(target, nil, []Configuration{{ID: uuid.New(), Spec: raw}}, nil, at.Add(time.Minute))
	if state != "pending" {
		t.Fatal("silence did not expire")
	}
}
