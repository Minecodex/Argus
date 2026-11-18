package alerting

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

func TestPolicyLabelsReachObservedRoutingAndNoiseWithoutSpoofingIdentity(t *testing.T) {
	id := uuid.New()
	rule := Rule{ID: uuid.New(), Expression: "owned_metric", Operator: ">", Threshold: 0.5, Severity: "warning"}
	data := promql.Vector{{Metric: labels.FromMap(map[string]string{"argus_resource_id": id.String(), "team": "observed"}), T: 1, F: 1}}
	result, err := decodeObservations(queryengine.Result{Data: data, ResultType: "vector"}, rule, []string{"route"}, []uuid.UUID{id},
		map[string]string{"route": "operations", "team": "configured", "resource_id": "forged", "argus_resource_id": "forged"})
	if err != nil || len(result) != 1 || result[0].Labels["route"] != "operations" || result[0].Labels["team"] != "observed" || result[0].Labels["resource_id"] != id.String() {
		t.Fatal("metadata was lost or replaced measured identity", result, err)
	}
	instance := Instance{ID: uuid.New(), PolicyID: uuid.New(), Lifecycle: "firing", Severity: "warning", ResourceIDs: []uuid.UUID{id}, State: InstanceState{Labels: result[0].Labels}}
	at := time.Now().UTC()
	spec, _ := json.Marshal(SilenceSpec{Enabled: true, ResourceIDs: []uuid.UUID{id}, Matchers: []Matcher{{Key: "route", Operator: "=", Value: "operations"}}, StartsAt: at.Add(-time.Minute), EndsAt: at.Add(time.Minute), Reason: "owned metadata scope"})
	decision, _ := noiseDecision(instance, []Instance{instance}, []Configuration{{ID: uuid.New(), Version: 1, Spec: spec}}, nil, at)
	if decision != "muted" {
		t.Fatal("configured labels did not reach noise routing", decision)
	}
}
