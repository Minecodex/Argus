package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func testPublishedAPM(t *testing.T, ctx context.Context, service Service, runtime Runtime, actor Actor, workflow actionservice.Service, conn driver.Conn, table string, host, source uuid.UUID) {
	insert := func(serviceName string, kind uint8, offset int64) {
		t.Helper()
		at := time.Now().UTC().Add(-time.Minute)
		if err := conn.Exec(ctx, "INSERT INTO `"+table+"` (resource_id,source_id,source_revision,source_type,trace_id,span_id,service_name,operation,span_kind,status,start_time,end_time,duration_ns,resource_attributes,kafka_offset,expires_at) VALUES (?,?,1,'otlp',?,?,?,'GET /orders',?,'error',?,?,100000000,?,?,now64(3)+INTERVAL 1 HOUR)", host, source, uuid.NewString(), uuid.NewString(), serviceName, kind, at, at.Add(100*time.Millisecond), map[string]string{"service.instance.id": "instance-1", "deployment.environment.name": "prod"}, offset); err != nil {
			t.Fatal(err)
		}
	}
	insert("orders", 2, 1)
	spec := EmptySpec()
	spec.Variables = []Variable{{ID: "env", Name: "env", Multiple: true, IncludeAll: true, Default: Selection{Values: []string{"prod"}}, Query: CandidateQuery{Signal: "traces", SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Field: "resource_attributes.deployment.environment.name"}}}
	for i, kind := range []string{"apm_services", "apm_instances", "apm_endpoints", "apm_red", "apm_topology"} {
		builder := &Builder{Operation: kind, Filters: []Filter{{Field: "resource_attributes.deployment.environment.name", Operator: "=", Variable: "env"}}}
		if kind == "apm_red" {
			builder.BucketSeconds = 60
		}
		spec.Panels = append(spec.Panels, Panel{ID: kind, Title: kind, Signal: "traces", Type: kind, AuthoringMode: "builder", SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, ApplicableResourceTypes: []string{"host"}, Layout: Rectangle{Y: i * 10, W: 12, H: 8, MinW: 2, MinH: 2}, Targets: []Target{{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: builder}}}})
	}
	raw, _ := json.Marshal(spec)
	draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Received sample APM", Spec: raw})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	confirmation, err := workflow.Confirm(ctx, actor.SubjectID.String(), uuid.NewString(), actor.EnterpriseID, 1, false, preview.Action.ActionRef, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var published resource.ActionCommitResult
	extension := ActionExtension{}
	if err := runtime.Store.InTx(ctx, func(q *db.Queries) error {
		var e error
		published, e = service.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, extension.RevalidateAction, extension.CommitAction)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	execution, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{ResourceIDs: []uuid.UUID{host}})
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Panels) != 5 || execution.Partial {
		t.Fatalf("APM runtime unavailable: %+v", execution)
	}
	if execution.Variables["env"].All || execution.Variables["env"].Values[0] != "prod" {
		t.Fatal("APM lost its published environment selection")
	}
	for _, panel := range execution.Panels {
		if panel.Status != "success" || len(panel.Targets) != 1 || panel.Targets[0].ResultType != panel.ID || len(panel.Sources) != 1 || panel.Sources[0].ID != source {
			t.Fatalf("APM publication/shape lost: %+v", panel)
		}
		if panel.Targets[0].Meta.LatestSampleAt == nil || panel.Targets[0].Meta.SampleTimeBasis != "observed_query_samples" || panel.Targets[0].Meta.IngestionStatus != "unknown" {
			t.Fatalf("APM sample freshness missing: %s", panel.ID)
		}
	}
	insert("internal-only", 1, 2)
	spec.Panels = spec.Panels[:1]
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Filters = []Filter{{Field: "serviceName", Operator: "=", Value: "internal-only"}}
	report, sample, err := runtime.Verify(ctx, actor, spec)
	if err != nil || !report.Valid {
		t.Fatalf("missing sample capability became invalid configuration: %+v %v", report, err)
	}
	var status struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(sample, &status) != nil || status.Status != "warning" {
		t.Fatalf("missing request samples claimed verified success: %s", sample)
	}
	spec.Panels[0].Type = "apm_red"
	if _, _, err := runtime.Verify(ctx, actor, spec); !errors.Is(err, ErrInvalid) {
		t.Fatal("APM chart/query shape mismatch accepted")
	}
}
