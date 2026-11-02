package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestSampleQuantityValidationUsesOriginalAuthorizedMetadata(t *testing.T) {
	source := ResolvedSource{ID: uuid.New(), Revision: 1, ResourceType: "host", Type: "hostmetrics"}
	for _, test := range []struct {
		name, metric, unit, display string
		original, complete          bool
		foreign                     bool
		rejected                    bool
	}{
		{"ratio_as_percent", "system_memory_utilization", "1", "percent", true, true, false, true},
		{"ratio_formatter", "system_memory_utilization", "1", "percent_ratio", true, true, false, false},
		{"bytes_as_percent", "system_memory_usage", "By", "percent", true, true, false, true},
		{"bytes_as_ratio", "system_memory_usage", "By", "percent_ratio", true, true, false, true},
		{"raw_bytes", "system_memory_usage", "By", "bytes", true, true, false, false},
		{"explicit_derived_scaling", "system_memory_utilization", "1", "percent", false, true, false, false},
		{"unknown_dimensionless", "status_code", "1", "percent", true, true, false, false},
		{"incomplete_metadata", "system_memory_utilization", "1", "percent", true, false, false, false},
		{"foreign_metadata", "system_memory_utilization", "1", "percent", true, true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := source.Key()
			if test.foreign {
				key = uuid.NewString() + ":1"
			}
			spec := EmptySpec()
			spec.Panels = []Panel{{ID: "p", Type: "timeseries", Signal: "metrics", Unit: test.display}}
			sample := Execution{Panels: []PanelExecution{{ID: "p", Sources: []ResolvedSource{source}, Targets: []TargetExecution{{ID: "q", Meta: queryengine.QueryMeta{InputMetricsOrigin: metricmeta.Origin, InputMetricsComplete: test.complete, ResultIsOriginalMetric: test.original, InputMetrics: []metricmeta.Descriptor{{Name: test.metric, Type: "gauge", Unit: test.unit, SourceKey: key}}}}}}}}
			issues := sampleQuantityIssues(spec, sample)
			if (len(issues) > 0) != test.rejected {
				t.Fatal("unexpected quantity display decision", issues)
			}
			if len(issues) > 0 && (issues[0].Code != "QUANTITY_DISPLAY_MISMATCH" || issues[0].Path != "panels.p.unit") {
				t.Fatal("feedback lost its repairable unit path", issues)
			}
		})
	}
}

func TestSampleQuantityValidationRecordedAuthoringReplay(t *testing.T) {
	root := os.Getenv("ARGUS_PLAN11_AUTHORING_REPLAY_DIR")
	if root == "" {
		t.Skip("requires explicit immutable authoring tool exports")
	}
	for _, name := range []string{"plan11-authoring-run-01-tools.json", "plan11-authoring-run-02-tools.json"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var calls []struct {
			Tool, Status string
			Input        struct{ Category, Name string }
			Projection   struct {
				Summary struct {
					Spec       json.RawMessage
					Validation struct{ Valid bool }
					Sample     struct{ Execution *Execution }
				}
			}
		}
		if json.Unmarshal(raw, &calls) != nil {
			t.Fatal("invalid authoring export")
		}
		var spec Spec
		checked := 0
		for _, call := range calls {
			if call.Tool != "tool.invoke" || call.Status != "succeeded" || call.Input.Category != "dashboard" {
				continue
			}
			summary := call.Projection.Summary
			if len(summary.Spec) > 0 {
				spec, err = DecodeSpec(summary.Spec)
				if err != nil {
					t.Fatal(err)
				}
			}
			if call.Input.Name == "draft.validate" && summary.Validation.Valid && summary.Sample.Execution != nil {
				issues := sampleQuantityIssues(spec, *summary.Sample.Execution)
				if len(issues) == 0 {
					t.Fatal("actual R67 direct-value percent display mismatch was not detected", name)
				}
				checked++
			}
		}
		if checked == 0 {
			t.Fatal("no actual validated original authoring sample was replayed", name)
		}
		t.Log("Actual original authoring sample flags percentage display mismatch; historical verdict unchanged", name)
	}
}

func TestSampleQuantityValidationUsesExactKnownRecipeOnly(t *testing.T) {
	source := ResolvedSource{ID: uuid.New(), Revision: 1, ResourceType: "host", Type: "hostmetrics"}
	for _, test := range []struct {
		name, expression, unit string
		reject                 bool
	}{
		{"known_cpu_wrong_scale", `1-avg by(argus_resource_id)(system_cpu_utilization{state="idle"})`, "percent", true},
		{"known_cpu_correct", `1-avg by(argus_resource_id)(system_cpu_utilization{state="idle"})`, "percent_ratio", false},
		{"explicit_scaled_cpu", `100*(1-avg by(argus_resource_id)(system_cpu_utilization{state="idle"}))`, "percent", false},
		{"unknown_derived", `2*system_cpu_utilization{state="idle"}`, "percent", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := EmptySpec()
			spec.Panels = []Panel{{ID: "p", Type: "timeseries", Signal: "metrics", Unit: test.unit, Targets: []Target{{ID: "q", Language: "promql", QueryMode: "range", RangeStepPolicy: StepPolicy{Kind: "fixed", Seconds: 30}, SourceDefinition: Definition{DSL: &DSL{Expression: test.expression}}}}}}
			sample := Execution{Panels: []PanelExecution{{ID: "p", Sources: []ResolvedSource{source}, Targets: []TargetExecution{{ID: "q", Meta: queryengine.QueryMeta{InputMetricsOrigin: metricmeta.Origin, InputMetricsComplete: true, ResultIsOriginalMetric: false, InputMetrics: []metricmeta.Descriptor{{Name: "system_cpu_utilization", Type: "gauge", Unit: "1", SourceKey: source.Key()}}}}}}}}
			if (len(sampleQuantityIssues(spec, sample)) > 0) != test.reject {
				t.Fatal("known definition vs unknown derived units misclassified")
			}
		})
	}
}
