package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPostgresQueryEvidenceFactsVerifyReadsAndRejectStaleOrForgedSources(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	loop := Loop{Store: f.store, DeliverResults: func(context.Context, db.Run) error { return nil }}
	workspace := uuid.New()
	f.exec("INSERT INTO workspaces(id,enterprise_id,conversation_id,status,pvc_name,namespace,capacity_bytes,environment_version) VALUES($1,$2,$3,'ready',$4,'private-query-evidence-ns',2147483648,'private-runtime')", workspace, f.e, f.c, "private-pvc-"+workspace.String())
	sequence := 0
	persist := func(tool, source string, input, data map[string]any) uuid.UUID {
		t.Helper()
		sequence++
		step, call := uuid.New(), uuid.New()
		raw, _ := json.Marshal(input)
		f.exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,$4,'model_call','running')", step, f.r, f.e, sequence)
		f.exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status,authorization_scope) VALUES($1,$2,$3,$4,$5,$6,$7,$8,decode(repeat('00',32),'hex'),'dispatched',$9)", call, call.String(), f.e, f.r, step, tool, source, raw, scope)
		target := tool
		if tool == "tool.invoke" {
			target = "telemetry.promql.query"
		}
		_, err := loop.persistToolOutcome(ctx, f.run(), db.ToolCall{ID: call, StepID: step, EnterpriseID: f.e, RunID: f.r, ToolID: tool, Source: source, AuthorizationScope: scope}, toolruntime.Result{ToolID: target, Data: data}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return call
	}
	content := map[string]any{"status": "success", "actual": "source-measurement"}
	body, _ := json.Marshal(content)
	hash := sha256.Sum256(body)
	query := persist("tool.invoke", "argus", map[string]any{}, content)
	path := toolruntime.NativeQueryFile("telemetry.promql.query", query)
	ref := "result_" + query.String()
	f.exec("INSERT INTO workspace_files(id,enterprise_id,workspace_id,conversation_id,name,path,byte_size,content_hash,media_type,source_result_refs) VALUES($1,$2,$3,$4,'query.json',$5,$6,$7,'application/json',$8)", uuid.New(), f.e, workspace, f.c, path, len(body), hex.EncodeToString(hash[:]), []string{ref})
	check := func(wantStatus string, available bool) []queryEvidenceFile {
		t.Helper()
		facts, err := loop.queryEvidenceFacts(ctx, f.run())
		if err != nil {
			t.Fatal(err)
		}
		items := facts["items"].([]queryEvidenceFile)
		if len(items) != 1 || items[0].ReadStatus != wantStatus || items[0].Available != available || available && items[0].Path != "/workspace/"+path || !available && items[0].Path != "" {
			t.Fatal("query evidence state confused delivery/read/availability", items)
		}
		raw, _ := json.Marshal(facts)
		if strings.Contains(string(raw), "private-pvc") || strings.Contains(string(raw), "private-query-evidence-ns") || strings.Contains(string(raw), "source-measurement") {
			t.Fatal("evidence facts leaked storage identity or sample contents")
		}
		return items
	}
	check("not_verified_by_framework", true)
	// The real persisted native projection must match an actual current tool
	// message. JSONB/history reserialization is allowed; old summaries are not.
	stored, err := f.store.Queries.GetPersistedToolResult(ctx, db.GetPersistedToolResultParams{ToolCallID: query, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	visible := []modelprovider.Message{{Role: "tool", ToolCallID: query.String(), Content: string(stored.Projection)}}
	inlineFacts, err := loop.queryEvidenceFacts(ctx, f.run(), visible...)
	if err != nil || !inlineFacts["items"].([]queryEvidenceFile)[0].InlineOriginalInContext {
		t.Fatal("current small original not certified", err, inlineFacts)
	}
	assembled, err := loop.context(ctx, f.run(), db.AiModelRevision{ContextWindowTokens: 131072, MaxOutputTokens: 1024}, append([]modelprovider.Message{{Role: "system", Content: "Use current evidence."}}, visible...), "", nil, ContextSource{})
	if err != nil || !strings.Contains(assembled.Messages[1].Content, `"inline_original_in_context":true`) {
		t.Fatal("current inline fact did not reach actual model request", err)
	}
	sequence++
	modelStep, err := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: int32(sequence), StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("controlled actual assembled provider request"))
	requestHash := hex.EncodeToString(digest[:])
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	modelCall, err := f.store.Queries.CreateModelCall(ctx, db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: modelStep.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: digest[:], InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
	if err != nil {
		t.Fatal(err)
	}
	exposure := func(want bool) {
		t.Helper()
		facts, e := loop.queryEvidenceFacts(ctx, f.run())
		if e != nil || facts["items"].([]queryEvidenceFile)[0].InlineExposureVerified != want {
			t.Fatal("historical complete exposure incorrect", want, e)
		}
	}
	exposure(false) // A reserved call and current flag do not prove dispatch.
	if err := persistModelDispatch(ctx, f.store, db.MarkModelDispatchedParams{ID: modelCall.ID, EnterpriseID: f.e, ProjectionHash: digest[:], CapabilitySnapshot: []byte(`{}`)}, modelInlineQueryReceipts(assembled, requestHash)); err != nil {
		t.Fatal(err)
	}
	exposure(false) // An in-flight or unknown provider response is insufficient.
	f.exec("UPDATE model_calls SET status='failed' WHERE id=$1", modelCall.ID)
	exposure(false)
	f.exec("UPDATE model_calls SET status='succeeded',completed_at=now() WHERE id=$1", modelCall.ID)
	exposure(true)
	f.exec("UPDATE model_calls SET projection_hash=decode(repeat('11',32),'hex') WHERE id=$1", modelCall.ID)
	exposure(false) // Receipt must bind this exact provider request hash.
	f.exec("UPDATE model_calls SET projection_hash=$2 WHERE id=$1", modelCall.ID, digest[:])
	exposure(true)
	compacted, err := loop.queryEvidenceFacts(ctx, f.run(), modelprovider.Message{Role: "user", Content: string(stored.Projection)})
	if err != nil || compacted["items"].([]queryEvidenceFile)[0].InlineOriginalInContext {
		t.Fatal("historical/user certificate survived compaction", err)
	}
	if !compacted["items"].([]queryEvidenceFile)[0].InlineExposureVerified {
		t.Fatal("actual prior complete exposure lost after compaction")
	}
	persist("read", "sandbox", map[string]any{"path": "/workspace/" + path}, map[string]any{"path": "/workspace/" + path, "content": string(body), "byte_size": len(body), "start_line": 1, "partial": true})
	check("not_verified_by_framework", true)
	persist("read", "external_mcp", map[string]any{"path": "/workspace/" + path}, map[string]any{"path": "/workspace/" + path, "content": string(body), "byte_size": len(body), "start_line": 1, "partial": false})
	check("not_verified_by_framework", true)
	persist("read", "argus", map[string]any{"path": "/workspace/" + path}, map[string]any{"path": "/workspace/" + path, "content": string(body), "byte_size": len(body), "start_line": 1, "partial": false})
	check("not_verified_by_framework", true)
	persist("read", "sandbox", map[string]any{"path": "/workspace/" + path}, map[string]any{"path": "/workspace/" + path, "content": string(body) + "\n", "byte_size": len(body), "start_line": 1, "partial": false})
	check("complete_read_verified", true)
	// Use the real inference context assembly, so restart/compaction cannot
	// leave this fact solely in a transient tool response or old summary.
	messages, current, _, contextSource, err := loop.messages(ctx, f.run())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := loop.context(ctx, f.run(), db.AiModelRevision{ContextWindowTokens: 131072, MaxOutputTokens: 1024}, messages, current, nil, contextSource)
	if err != nil || len(projection.Messages) < 2 || !strings.Contains(projection.Messages[1].Content, "complete_read_verified") || !strings.Contains(projection.Messages[1].Content, query.String()) {
		t.Fatal("current query evidence facts did not reach the actual assembled model request", err)
	}
	f.exec("UPDATE workspace_files SET content_hash=repeat('1',64) WHERE workspace_id=$1 AND path=$2", workspace, path)
	check("not_verified_by_framework", false)
	inlineFacts, err = loop.queryEvidenceFacts(ctx, f.run(), visible...)
	if err != nil || inlineFacts["items"].([]queryEvidenceFile)[0].InlineOriginalInContext {
		t.Fatal("modified copy retained inline availability", err)
	}
	f.exec("UPDATE workspace_files SET content_hash=$3 WHERE workspace_id=$1 AND path=$2", workspace, path, hex.EncodeToString(hash[:]))
	check("complete_read_verified", true)
	f.exec("UPDATE workspaces SET status='deleting' WHERE id=$1", workspace)
	check("not_verified_by_framework", false)
	f.exec("UPDATE workspaces SET status='ready' WHERE id=$1", workspace)
	// External same-named output is never a native-query ledger entry.
	persist("tool.invoke", "external_mcp", map[string]any{}, content)
	check("complete_read_verified", true)
	otherRun := f.run()
	otherRun.ID = uuid.New()
	otherFacts, err := loop.queryEvidenceFacts(ctx, otherRun)
	if err != nil || len(otherFacts["items"].([]queryEvidenceFile)) != 0 {
		t.Fatal("query evidence crossed Run identity", otherFacts, err)
	}
	for i := 0; i < 65; i++ {
		persist("tool.invoke", "argus", map[string]any{}, content)
	}
	bounded, err := loop.queryEvidenceFacts(ctx, f.run())
	if err != nil || len(bounded["items"].([]queryEvidenceFile)) != 64 || bounded["complete"] != false {
		t.Fatal("native query evidence ledger grew without a stated bound", bounded, err)
	}
	f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.u)
	facts, err := loop.queryEvidenceFacts(ctx, f.run())
	if err != nil || len(facts["items"].([]queryEvidenceFile)) != 0 || facts["complete"] != true {
		t.Fatal("old evidence identity/path/hash survived current authorization change", facts, err)
	}
	t.Log("Actual PostgreSQL evidence facts: complete original read only, partial/external/edited/unavailable/current-authorization cases preserved")
}
