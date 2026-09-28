package dashboard

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func testPublishedConversion(t *testing.T, ctx context.Context, service Service, runtime Runtime, actor Actor, workflow actionservice.Service, conn driver.Conn, tables telemetry.TenantTables, host, source uuid.UUID) {
	if _, err := runtime.Store.Pool.Exec(ctx, `UPDATE telemetry_sources SET signals=ARRAY['metrics','logs','traces'] WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	from, to := at.Add(-5*time.Minute), at.Add(time.Minute)
	series := uuid.New()
	if err := conn.Exec(ctx, "INSERT INTO `"+tables.MetricSeries+"` (resource_id,series_id,metric_name,labels,metric_type,source_id,source_revision,source_type,expires_at) VALUES (?,?,'conversion_cpu',?,'gauge',?,1,'otlp',now64(3)+INTERVAL 1 HOUR)", host, series, map[string]string{"service": "orders"}, source); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "INSERT INTO `"+tables.MetricSamples+"` (resource_id,series_id,metric_name,timestamp,value,float_value,sample_type,ingest_key,source_id,source_revision,source_type,expires_at) VALUES (?,?,'conversion_cpu',?,23,23,'float','conversion',?,1,'otlp',now64(3)+INTERVAL 1 HOUR)", host, series, at, source); err != nil {
		t.Fatal(err)
	}
	spec := EmptySpec()
	spec.DefaultTimeRange = TimeRange{Kind: "absolute", From: &from, To: &to}
	binding := SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	spec.Variables = []Variable{{ID: "entry", Name: "entry", IncludeAll: true, Default: Selection{Values: []string{"authorized evidence"}}, Query: CandidateQuery{Signal: "logs", SourceBinding: binding, Field: "body"}}}
	metric := validPanel()
	metric.ID, metric.Title = "metric", "Converted metric"
	metric.SourceBinding = binding
	metric.Targets[0].SourceDefinition = Definition{Builder: &Builder{Operation: "sum", Metric: "conversion_cpu", Filters: []Filter{{Field: "service", Operator: "=", Variable: "entry"}}}}
	metric.Targets[0].ParameterBindings = []ParameterBinding{{Parameter: "service", Variable: "entry", ValueMap: map[string]string{"authorized evidence": "orders"}}}
	logs := metric
	logs.ID, logs.Title, logs.Signal, logs.Type = "logs", "Converted logs", "logs", "logs"
	logs.Layout.Y = 10
	logs.Targets = []Target{{ID: "a", Language: queryengine.LanguageKQL, SourceDefinition: Definition{Builder: &Builder{Operation: "records", Limit: 5, Filters: []Filter{{Field: "body", Operator: "=", Variable: "entry"}}}}}}
	traces := logs
	traces.ID, traces.Title, traces.Signal, traces.Type = "traces", "Converted traces", "traces", "trace_list"
	traces.Layout.Y = 20
	traces.Targets = []Target{{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: &Builder{Operation: "list", Limit: 10, Filters: []Filter{{Field: "serviceName", Operator: "=", Value: "frontend"}}}}}}
	apm := traces
	apm.ID, apm.Type, apm.Title = "apm", "apm_services", "Converted APM"
	apm.Layout.Y = 30
	apm.Targets = []Target{{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: &Builder{Operation: "apm_services", Filters: []Filter{{Field: "serviceName", Operator: "=", Variable: "entry"}}}}, ParameterBindings: metric.Targets[0].ParameterBindings}}
	spec.Panels = []Panel{metric, logs, traces, apm}
	seedErrorCounters(t, ctx, conn, tables, host, source, at)
	ratio := metric
	ratio.ID, ratio.Title, ratio.Unit, ratio.Layout.Y = "ratio", "Received request error ratio", "percent_ratio", 40
	ratio.Targets = []Target{{ID: "a", Language: queryengine.LanguagePromQL, QueryMode: "range", RangeStepPolicy: StepPolicy{Kind: "fixed", Seconds: 30}, SourceDefinition: Definition{Builder: &Builder{Operation: "error_rate", Metric: "planv2_requests_total", MetricType: "counter", WindowSeconds: 300, GroupBy: []string{"service"}, ErrorFilters: []Filter{{Field: "status", Operator: "=~", Value: "5.."}}}}}}
	spec.Panels = append(spec.Panels, ratio)
	for _, panel := range spec.Panels {
		generated, err := GenerateStandardDrilldowns(spec, panel.ID, nil)
		if err != nil || len(generated.Issues) > 0 {
			t.Fatalf("generate: %v %v", generated.Issues, err)
		}
		spec = generated.Spec
	}
	publish := func(id uuid.UUID, spec Spec) uuid.UUID {
		t.Helper()
		raw, _ := json.Marshal(spec)
		draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Conversion", DashboardID: id, Spec: raw})
		if err != nil {
			t.Fatal(err)
		}
		if id != uuid.Nil {
			draft, err = service.SaveDraft(ctx, actor, draft.ID, DraftInput{Name: draft.Name, Spec: raw, ExpectedVersion: draft.DraftVersion})
			if err != nil {
				t.Fatal(err)
			}
		}
		preview, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		confirmation, err := workflow.Confirm(ctx, actor.SubjectID.String(), uuid.NewString(), actor.EnterpriseID, actor.AuthorizationVersion, false, preview.Action.ActionRef, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		var committed resource.ActionCommitResult
		extension := ActionExtension{}
		if err := runtime.Store.InTx(ctx, func(q *db.Queries) error {
			var err error
			committed, err = service.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, extension.RevalidateAction, extension.CommitAction)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return committed.ResourceID
	}
	id := publish(uuid.Nil, spec)
	input := ExecutionInput{From: &from, To: &to, ResourceIDs: []uuid.UUID{host}}
	baseline, err := runtime.Execute(ctx, actor, id, input)
	if err != nil || baseline.Partial {
		t.Fatalf("baseline: %+v %v", baseline, err)
	}
	for _, panel := range baseline.Panels {
		if panel.Status != "success" {
			t.Fatalf("baseline has no evidence: %+v", panel)
		}
	}
	baselineJSON := func(e Execution) string {
		values := []any{}
		for _, p := range e.Panels {
			for _, target := range p.Targets {
				values = append(values, struct {
					Type string
					Data any
				}{target.ResultType, target.Data})
			}
		}
		raw, _ := json.Marshal(values)
		return string(raw)
	}(baseline)
	for _, mode := range []string{"display", "dsl", "builder"} {
		if mode == "display" {
			min, max := 0.0, 100.0
			spec.Panels[0].Display = &DisplayOptions{DrawStyle: "area", Stack: true, Smooth: true, Min: &min, Max: &max}
			spec.Panels[0].Thresholds = []Threshold{{Value: 20, Tone: "warning"}, {Value: 80, Tone: "danger"}}
		} else {
			for i, panel := range spec.Panels {
				result := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: mode})
				if !result.Converted {
					t.Fatal(result.Issues)
				}
				spec.Panels[i] = result.Panel
			}
		}
		publish(id, spec)
		_, revision, err := service.Get(ctx, actor, id)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := DecodeSpec(revision.Spec)
		if err != nil || !reflect.DeepEqual(restored.Panels[0].Display, spec.Panels[0].Display) || !reflect.DeepEqual(restored.Panels[0].Thresholds, spec.Panels[0].Thresholds) {
			t.Fatalf("display configuration lost at publication: %+v %v", restored, err)
		}
		after, err := runtime.Execute(ctx, actor, id, input)
		if err != nil || after.Partial {
			t.Fatalf("converted execution: %+v %v", after, err)
		}
		if mode == "display" && after.Panels[0].Targets[0].QueryHash != baseline.Panels[0].Targets[0].QueryHash {
			t.Fatal("presentation edit changed the executed query hash")
		}
		values := []any{}
		for _, p := range after.Panels {
			for _, target := range p.Targets {
				values = append(values, struct {
					Type string
					Data any
				}{target.ResultType, target.Data})
			}
		}
		raw, _ := json.Marshal(values)
		if string(raw) != baselineJSON {
			t.Fatalf("%s changed observations:\n%s\n%s", mode, baselineJSON, raw)
		}
		// A standard trace detail remains executable after the whole-panel conversion.
		for _, drill := range spec.Panels[2].Drilldowns {
			if drill.OriginQueryRef == "a" && drill.Kind == "trace_details" {
				detail, err := runtime.Drilldown(ctx, actor, id, DrilldownInput{ContextToken: after.ContextToken, PanelID: "traces", DrilldownID: drill.ID, Values: map[string]string{"trace_id": hex.EncodeToString([]byte("drilltrace1234567")), "source_id": source.String(), "resource_id": host.String()}})
				if err != nil || detail.Result.Status != "success" {
					t.Fatalf("converted detail: %+v %v", detail, err)
				}
			}
		}
	}
}
