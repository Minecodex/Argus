package agent

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestHardCompactionMeasuresActualDurableNextTurn(t *testing.T) {
	f := newRecoveryFixture(t)
	f.exec("INSERT INTO ai_model_revisions(id,model_id,enterprise_id,revision,base_url,provider_model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities) SELECT gen_random_uuid(),id,enterprise_id,revision,base_url,model_id,api_protocol,262144,max_output_tokens,input_price_per_million,output_price_per_million,capabilities FROM ai_models WHERE id=$1", f.m)
	f.exec(`UPDATE runs SET status='waiting_system',stop_reason='context_compaction',verification_only=true,tool_snapshot='{"version":"argus.model_tool_set/v1","tools":[]}' WHERE id=$1`, f.r)
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	for i := 0; i < 21; i++ {
		step, err := f.store.Queries.CreateRunStep(t.Context(), db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: int32(i + 1), StepType: "model_call", Status: "succeeded"})
		if err != nil {
			t.Fatal(err)
		}
		call, err := f.store.Queries.CreateModelCall(t.Context(), db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: bytes.Repeat([]byte{0x42}, 32), InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
		if err != nil {
			t.Fatal(err)
		}
		f.exec("UPDATE model_calls SET status='succeeded',dispatched_at=now(),completed_at=now() WHERE id=$1", call.ID)
	}
	run := f.run()
	loop := Loop{Store: f.store}
	messages, current, _, source, err := loop.messages(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := f.store.Queries.GetEnabledAIModelRevision(t.Context(), db.GetEnabledAIModelRevisionParams{ModelID: f.m, EnterpriseID: f.e, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	provider := modelprovider.Provider{Protocol: modelprovider.Protocol(revision.ApiProtocol), BaseURL: revision.BaseUrl}
	base, err := loop.context(t.Context(), run, revision, messages, current, nil, source)
	if err != nil {
		t.Fatal(err)
	}
	legacySize, err := provider.RequestBytes(modelprovider.Request{Model: revision.ProviderModelID, MaxTokens: int(revision.MaxOutputTokens), Messages: base.Messages})
	if err != nil {
		t.Fatal(err)
	}
	turnMessages, turnTools, err := inferenceTurnContext(run.ModeSnapshot, 21, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := loop.context(t.Context(), run, revision, turnMessages, current, turnTools, source)
	if err != nil {
		t.Fatal(err)
	}
	parameter, err := modelprovider.BudgetParameterForCapabilities(provider.Protocol, revision.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	effort, err := modelprovider.ReasoningEffortForCapabilities(provider.Protocol, revision.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	actualSize, err := provider.RequestBytes(inferenceRequest(revision.ProviderModelID, int(revision.MaxOutputTokens), actual.Messages, turnTools, parameter, effort))
	if err != nil || actualSize <= legacySize {
		t.Fatal("fixture did not expose the actual next-turn overhead", legacySize, actualSize, err)
	}
	// Choose a real model budget boundary that accepts the former base request
	// but rejects the next durable inference request. No tokenizer estimate is used.
	window := 32768
	for modelprovider.NewContextBudget(window, int(revision.MaxOutputTokens)).HardLimit <= legacySize {
		window++
		if window > 262144 {
			t.Fatal("fixture request exceeds bounded model window")
		}
	}
	hard := modelprovider.NewContextBudget(window, int(revision.MaxOutputTokens)).HardLimit
	if actualSize < hard {
		t.Fatal("fixture failed to straddle the real hard limit", legacySize, actualSize, hard)
	}
	f.exec("UPDATE ai_models SET context_window_tokens=$2 WHERE id=$1", f.m, window)
	f.exec("UPDATE ai_model_revisions SET context_window_tokens=$2 WHERE model_id=$1", f.m, window)
	compactor := Compactor{Store: f.store}
	if fits, err := compactor.currentContextFits(t.Context(), run); err != nil || fits {
		t.Fatal("oversized next turn was incorrectly resumed", fits, err)
	}
	if err := compactor.Handle(t.Context(), f.task("compaction", "hard_limit")); err != nil {
		t.Fatal(err)
	}
	if next := f.run(); next.Status != "failed" || next.ErrorCode.String != "CONTEXT_COMPACTION_FAILED" {
		t.Fatal("unshrinkable next turn did not terminate", next.Status, next.ErrorCode)
	}
	if f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND queue='agent'", f.r) != 0 {
		t.Fatal("hard-limit recovery requeued an unchanged oversize inference")
	}
	t.Logf("legacy base bytes=%d < hard=%d <= actual next-turn bytes=%d; no provider call or agent resume", legacySize, hard, actualSize)
}
