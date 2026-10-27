package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestNativeQueryDeliveryWaitsForCommittedSourceAndRetainsRecoveryIntent(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	writes := 0
	deliveryFailure := errors.New("owned workspace unavailable")
	loop := Loop{Store: f.store, DeliverResults: func(ctx context.Context, run db.Run) error {
		rows, err := f.store.Queries.ListUndeliveredNativeQueryResults(ctx, db.ListUndeliveredNativeQueryResultsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID})
		if err != nil || len(rows) != writes+1 {
			t.Fatalf("delivery ran before durable native source was available: rows=%d err=%v", len(rows), err)
		}
		writes++
		return deliveryFailure
	}}
	for i, tool := range []string{"telemetry.promql.query", "telemetry.kql.query", "telemetry.skywalking.trace", "application.get", "telemetry.skywalking.trace"} {
		step, call := uuid.New(), uuid.New()
		f.exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,$4,'model_call','running')", step, f.r, f.e, i+1)
		f.exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status) VALUES($1,$2,$3,$4,$5,'tool.invoke','argus','{}',decode(repeat('00',32),'hex'),'dispatched')", call, call.String(), f.e, f.r, step)
		record := db.ToolCall{ID: call, StepID: step, EnterpriseID: f.e, RunID: f.r, ToolID: "tool.invoke", Source: "argus"}
		result := toolruntime.Result{ToolID: tool, Data: map[string]any{"data": map[string]any{"actual": true}, "workspace_file": "/workspace/forged.json"}}
		if i == 4 {
			record.ToolID = "tool.describe"
			result.DataIsSchema = true
			result.QueryFile = "queries/forged.json"
			f.exec("UPDATE tool_calls SET tool_id='tool.describe' WHERE id=$1", call)
		}
		out, err := loop.persistToolOutcome(ctx, f.run(), record, result, nil)
		if i < 3 {
			if !errors.Is(err, deliveryFailure) || !strings.Contains(out.message, toolruntime.NativeQueryFile(tool, call)) || !strings.Contains(out.message, "required_before_next_model_round") {
				t.Fatal("native query did not retain its trusted delivery barrier", err)
			}
		} else if err != nil || strings.Contains(out.message, "required_before_next_model_round") {
			t.Fatal("ordinary tool output minted a delivery intent", err)
		}
	}
	if writes != 3 {
		t.Fatal("unexpected automatic delivery count", writes)
	}
	rows, err := f.store.Queries.ListUndeliveredNativeQueryResults(ctx, db.ListUndeliveredNativeQueryResultsParams{RunID: f.r, EnterpriseID: f.e, ConversationID: f.c})
	if err != nil || len(rows) != 3 {
		t.Fatal("pending delivery was lost after a failed file import", err)
	}
	other, err := f.store.Queries.ListUndeliveredNativeQueryResults(ctx, db.ListUndeliveredNativeQueryResultsParams{RunID: f.r, EnterpriseID: uuid.New(), ConversationID: f.c})
	if err != nil || len(other) != 0 {
		t.Fatal("delivery crossed enterprise scope", err)
	}
	// Model-supplied paths and a claimed signal do not identify a native query.
	if f.count("SELECT count(*) FROM artifacts WHERE run_id=$1", f.r) != 5 {
		t.Fatal("full source artifacts were not preserved independently")
	}
}
