package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func drillSpec(mode string) Spec {
	spec := EmptySpec()
	panel := validPanel()
	panel.Signal, panel.Type = "traces", "apm_services"
	panel.SourceBinding = SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	panel.AuthoringMode = mode
	target := Target{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: &Builder{Operation: "apm_services", Filters: []Filter{{Field: "resource_attributes.env", Operator: "=", Value: "prod"}, {Field: "serviceInstanceName", Operator: "=", Value: "instance-1"}}}}}
	if mode == "dsl" {
		query, _ := compileAPMBuilder(*target.SourceDefinition.Builder)
		target.SourceDefinition = Definition{DSL: &query}
	}
	panel.Targets = []Target{target}
	spec.Panels = []Panel{panel}
	return spec
}

func TestStandardDrilldownsKeepConstraintsAndEditingSources(t *testing.T) {
	for _, mode := range []string{"builder", "dsl"} {
		t.Run(mode, func(t *testing.T) {
			spec := drillSpec(mode)
			before, _ := json.Marshal(spec)
			result, err := GenerateStandardDrilldowns(spec, "cpu", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Added) < 4 || len(result.Issues) > 0 {
				t.Fatalf("standard trace/log actions missing: %+v", result)
			}
			if report := Validate(result.Spec); !report.Valid {
				t.Fatal(report.Issues)
			}
			after, _ := json.Marshal(spec)
			if !bytes.Equal(before, after) {
				t.Fatal("generation modified caller's query")
			}
			found := false
			for _, d := range result.Spec.Panels[0].Drilldowns {
				if d.Kind != "service_traces" {
					continue
				}
				target, _ := findTarget(result.Spec.Panels[0], d.DetailQueryRef)
				bound, e := bindTargetInputs(result.Spec, result.Spec.Panels[0], target, nil, nil, map[string]Selection{"service": {Values: []string{"api"}}, "source": {Values: []string{uuid.NewString()}}, "resource": {Values: []string{uuid.NewString()}}})
				if e != nil {
					t.Fatal(e)
				}
				query, e := compileConcreteTarget(bound, false)
				if e != nil {
					t.Fatal(e)
				}
				if !strings.Contains(query.Query.Expression, "prod") || !strings.Contains(query.Query.Expression, "instance-1") {
					t.Fatalf("derivation discarded original constraints: %s", query.Query.Expression)
				}
				found = true
			}
			if !found {
				t.Fatal("trace list drilldown missing")
			}
			second, err := GenerateStandardDrilldowns(result.Spec, "cpu", nil)
			if err != nil || len(second.Added) > 0 {
				t.Fatal("regeneration replaced published/customizable definitions")
			}
		})
	}
}

func TestDrilldownContextAndRowProofBoundaries(t *testing.T) {
	runtime := Runtime{ContextKey: bytes.Repeat([]byte{9}, 32)}
	actor := Actor{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user"}
	value := executionContext{Version: contextVersion, Enterprise: actor.EnterpriseID, Subject: actor.SubjectID, SubjectType: actor.SubjectType, ExpiresAt: time.Now().Add(time.Minute)}
	token, err := runtime.signContext(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.readContext(actor, token); err != nil {
		t.Fatal(err)
	}
	other := actor
	other.SubjectID = uuid.New()
	if _, err := runtime.readContext(other, token); !errors.Is(err, ErrDenied) {
		t.Fatal("context crossed subjects")
	}
	if _, err := runtime.readContext(actor, "a"+token[1:]); !errors.Is(err, ErrDenied) {
		t.Fatal("tampered scope accepted")
	}
	value.ExpiresAt = time.Now().Add(-time.Minute)
	expired, _ := runtime.signContext(value)
	if _, err := runtime.readContext(actor, expired); !errors.Is(err, ErrContextExpired) {
		t.Fatal("expired context accepted")
	}
	drill := Drilldown{Inputs: map[string]string{"id": "/traceId", "resource": "/resourceId"}}
	data := map[string]any{"queryTraces": map[string]any{"traces": []any{map[string]any{"traceId": "one", "resourceId": "a"}, map[string]any{"traceId": "two", "resourceId": "b"}}}}
	if ok, err := matchDrilldownRow(data, drill, map[string]string{"id": "one", "resource": "a"}); err != nil || !ok {
		t.Fatal("visible row not matched")
	}
	if ok, _ := matchDrilldownRow(data, drill, map[string]string{"id": "one", "resource": "b"}); ok {
		t.Fatal("fields from different rows were combined")
	}
	if _, err := matchDrilldownRow(data, drill, map[string]string{"id": "one", "resource": "a", "query": "override"}); err == nil {
		t.Fatal("undeclared runtime input accepted")
	}
}

func TestDrillWindowClipsToOriginalRange(t *testing.T) {
	from := time.Date(2026, 9, 25, 0, 0, 5, 0, time.UTC)
	scope := queryScope{From: from, To: from.Add(10 * time.Second)}
	d := Drilldown{TimeWindow: &DrilldownTimeWindow{Input: "at", DurationInput: "seconds"}}
	window, err := applyDrillWindow(scope, d, map[string]string{"at": from.Add(-5 * time.Second).Format(time.RFC3339Nano), "seconds": "10"})
	if err != nil || !window.From.Equal(from) || !window.To.Equal(from.Add(5*time.Second)) {
		t.Fatal("drill window expanded original time")
	}
	if _, err := applyDrillWindow(scope, d, map[string]string{"at": from.Format(time.RFC3339Nano), "seconds": "NaN"}); err == nil {
		t.Fatal("nonfinite duration accepted")
	}
}

func TestREDDrilldownUsesObservedBucket(t *testing.T) {
	spec := drillSpec("builder")
	spec.Panels[0].Type = "apm_red"
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Operation = "apm_red"
	spec.Panels[0].Targets[0].SourceDefinition.Builder.BucketSeconds = 60
	result, err := GenerateStandardDrilldowns(spec, "cpu", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, drill := range result.Spec.Panels[0].Drilldowns {
		if drill.Kind == "service_traces" {
			if drill.TimeWindow == nil || drill.TimeWindow.Input != "bucket_start" || drill.TimeWindow.DurationInput != "bucket_seconds" {
				t.Fatal("RED drilldown widened to the whole dashboard window")
			}
			return
		}
	}
	t.Fatal("RED drilldown not generated")
}
