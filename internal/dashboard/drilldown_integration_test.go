package dashboard

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
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

func testPublishedDrilldowns(t *testing.T, ctx context.Context, service Service, runtime Runtime, actor Actor, workflow actionservice.Service, conn driver.Conn, tables telemetry.TenantTables, a, b, c, sourceA, sourceB, sourceC uuid.UUID) {
	grant := uuid.New()
	if _, err := runtime.Store.Pool.Exec(ctx, `INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)`, grant, actor.EnterpriseID, actor.SubjectID, b); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = runtime.Store.Pool.Exec(context.Background(), `DELETE FROM data_authorization_grants WHERE id=$1`, grant)
	})
	at := time.Now().UTC().Add(-time.Minute)
	traceID := hex.EncodeToString([]byte("drilltrace1234567"))
	rootID, childID, hiddenID := hex.EncodeToString([]byte("rootspan")), hex.EncodeToString([]byte("childspn")), hex.EncodeToString([]byte("hiddens1"))
	for i, row := range []struct {
		resource, source    uuid.UUID
		id, parent, service string
	}{{a, sourceA, rootID, "", "frontend"}, {b, sourceB, childID, rootID, "backend"}, {c, sourceC, hiddenID, childID, "hidden-service"}} {
		if err := conn.Exec(ctx, "INSERT INTO `"+tables.Traces+"` (resource_id,source_id,source_revision,source_type,trace_id,span_id,parent_span_id,service_name,operation,span_kind,status,start_time,end_time,duration_ns,kafka_offset,expires_at) VALUES (?,?,1,'otlp',?,?,?,?,'request',2,'ok',?,?,100000000,?,now64(3)+INTERVAL 1 HOUR)", row.resource, row.source, traceID, row.id, row.parent, row.service, at, at.Add(100*time.Millisecond), int64(100+i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,source_id,source_revision,source_type,timestamp,body,trace_id,span_id,event_id,expires_at) VALUES (?,?,1,'otlp',?,'correlated evidence',?,?,?,now64(3)+INTERVAL 1 HOUR)", b, sourceB, at, traceID, childID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	spec := EmptySpec()
	panel := validPanel()
	panel.ID, panel.Type, panel.Signal = "traces", "trace_list", "traces"
	panel.SourceBinding = SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	panel.Targets = []Target{{ID: "list", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: &Builder{Operation: "list", Limit: 100}}}}
	spec.Panels = []Panel{panel}
	encoded, _ := json.Marshal(spec)
	draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Drilldown authorization", Spec: encoded})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := service.GenerateDrilldowns(ctx, actor, draft.ID, GenerateDrilldownsInput{ExpectedVersion: draft.DraftVersion, PanelID: panel.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(generated.Added) == 0 || generated.Draft.DraftVersion != draft.DraftVersion+1 {
		t.Fatal("standard generation was not saved as a personal draft")
	}
	draft = generated.Draft
	spec, err = DecodeSpec(draft.Spec)
	if err != nil {
		t.Fatal(err)
	}
	publish := func(draft db.DashboardDraft) uuid.UUID {
		t.Helper()
		preview, e := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		confirmation, e := workflow.Confirm(ctx, actor.SubjectID.String(), uuid.NewString(), actor.EnterpriseID, 1, false, preview.Action.ActionRef, uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		var result resource.ActionCommitResult
		extension := ActionExtension{}
		if e := runtime.Store.InTx(ctx, func(q *db.Queries) error {
			var e error
			result, e = service.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, extension.RevalidateAction, extension.CommitAction)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		return result.ResourceID
	}
	id := publish(draft)
	execution, err := runtime.Execute(ctx, actor, id, ExecutionInput{ResourceIDs: []uuid.UUID{a}})
	if err != nil {
		t.Fatal(err)
	}
	if execution.ContextToken == "" {
		t.Fatal("execution context was not issued")
	}
	find := func(origin, kind string) Drilldown {
		t.Helper()
		for _, d := range spec.Panels[0].Drilldowns {
			if d.OriginQueryRef == origin && d.Kind == kind {
				return d
			}
		}
		t.Fatalf("missing %s from %s", kind, origin)
		return Drilldown{}
	}
	details := find("list", "trace_details")
	selected := map[string]string{"trace_id": traceID, "source_id": sourceA.String(), "resource_id": a.String()}
	ordinary, err := runtime.Drilldown(ctx, actor, id, DrilldownInput{ContextToken: execution.ContextToken, PanelID: panel.ID, DrilldownID: details.ID, Values: selected})
	if err != nil {
		t.Fatal(err)
	}
	assertSpans := func(value DrilldownExecution, count int) {
		t.Helper()
		if value.Result.Status != "success" {
			t.Fatalf("detail query failed: %+v", value.Result)
		}
		data := value.Result.Data.(map[string]any)["queryTraceGraph"].(map[string]any)
		if len(data["spans"].([]any)) != count {
			t.Fatalf("wrong expansion: %+v", data)
		}
		raw, _ := json.Marshal(value.Result.Data)
		if strings.Contains(string(raw), "hidden-service") {
			t.Fatal("unauthorized resource returned")
		}
	}
	assertSpans(ordinary, 1)
	full := find(details.DetailQueryRef, "full_trace")
	fullInput := DrilldownInput{ContextToken: ordinary.ContextToken, PanelID: panel.ID, DrilldownID: full.ID, Values: selected}
	if _, err := runtime.Drilldown(ctx, actor, id, fullInput); !errors.Is(err, ErrInvalid) {
		t.Fatal("full expansion happened without explicit opt-in")
	}
	fullInput.ExpandAuthorizedResources = true
	expanded, err := runtime.Drilldown(ctx, actor, id, fullInput)
	if err != nil {
		t.Fatal(err)
	}
	assertSpans(expanded, 2)
	logs := find(full.DetailQueryRef, "span_logs")
	logInput := DrilldownInput{ContextToken: expanded.ContextToken, PanelID: panel.ID, DrilldownID: logs.ID, Values: map[string]string{"trace_id": traceID, "span_id": childID, "resource_id": b.String(), "source_id": sourceB.String()}}
	correlated, err := runtime.Drilldown(ctx, actor, id, logInput)
	if err != nil {
		t.Fatal(err)
	}
	if correlated.Result.Status != "success" {
		t.Fatalf("published related logs failed: %+v", correlated.Result)
	}
	logRows := correlated.Result.Data.([]map[string]any)
	if len(logRows) != 1 || logRows[0]["body"] != "correlated evidence" {
		t.Fatal("log correlation did not preserve span identity")
	}
	contextDrill := find(logs.DetailQueryRef, "log_context")
	contextResult, err := runtime.Drilldown(ctx, actor, id, DrilldownInput{ContextToken: correlated.ContextToken, PanelID: panel.ID, DrilldownID: contextDrill.ID, Values: map[string]string{"event": logRows[0]["event_id"].(string)}})
	if err != nil || contextResult.Result.Status != "success" {
		t.Fatalf("published log context failed: %+v %v", contextResult, err)
	}
	forged := DrilldownInput{ContextToken: execution.ContextToken, PanelID: panel.ID, DrilldownID: details.ID, Values: map[string]string{"trace_id": "not-in-the-published-result", "source_id": sourceA.String(), "resource_id": a.String()}}
	if _, err := runtime.Drilldown(ctx, actor, id, forged); !errors.Is(err, ErrSelectionStale) {
		t.Fatalf("forged row accepted: %v", err)
	}
	t.Run("published file drilldown chain", func(t *testing.T) {
		testPublishedFileDrilldowns(t, ctx, runtime, actor, id, a, b, c, grant, spec, traceID, childID, selected)
	})
	if _, err := runtime.Store.Pool.Exec(ctx, `DELETE FROM data_authorization_grants WHERE id=$1`, grant); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Drilldown(ctx, actor, id, logInput); !errors.Is(err, ErrDenied) {
		t.Fatalf("revoked resource reused a context: %v", err)
	}
	afterRevocation, err := runtime.Drilldown(ctx, actor, id, fullInput)
	if err != nil {
		t.Fatal(err)
	}
	assertSpans(afterRevocation, 1)
	// A new publication does not reinterpret a still-valid original execution.
	edit, err := service.CreateDraft(ctx, actor, DraftInput{DashboardID: id})
	if err != nil {
		t.Fatal(err)
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Filters = []Filter{{Field: "serviceName", Operator: "=", Value: "not-received"}}
	encoded, _ = json.Marshal(spec)
	edit, err = service.SaveDraft(ctx, actor, edit.ID, DraftInput{Name: edit.Name, Description: edit.Description, Spec: encoded, ExpectedVersion: edit.DraftVersion})
	if err != nil {
		t.Fatal(err)
	}
	publish(edit)
	stillOld, err := runtime.Drilldown(ctx, actor, id, DrilldownInput{ContextToken: execution.ContextToken, PanelID: panel.ID, DrilldownID: details.ID, Values: selected})
	if err != nil || stillOld.RevisionID != execution.RevisionID {
		t.Fatalf("old execution mixed in a new revision: %v", err)
	}
	fresh, err := runtime.Execute(ctx, actor, id, ExecutionInput{ResourceIDs: []uuid.UUID{a}})
	if err != nil || fresh.RevisionID == execution.RevisionID || fresh.Panels[0].Status != "no_data" {
		t.Fatal("new execution did not select the latest publication")
	}
}
