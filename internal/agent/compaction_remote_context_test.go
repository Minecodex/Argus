package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestHardCompactionMeasuresRemoteFactsAndDoesNotResumeUnchangedOversizeInput(t *testing.T) {
	f := newRecoveryFixture(t)
	f.exec("INSERT INTO ai_model_revisions(id,model_id,enterprise_id,revision,base_url,provider_model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities) SELECT gen_random_uuid(),id,enterprise_id,revision,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities FROM ai_models WHERE id=$1", f.m)
	f.exec(`UPDATE runs SET status='waiting_system',stop_reason='context_compaction',verification_only=true,tool_snapshot='{"version":"argus.model_tool_set/v1","tools":[]}' WHERE id=$1`, f.r)
	compactor := Compactor{Store: f.store}
	if fits, err := compactor.currentContextFits(t.Context(), f.run()); err != nil || !fits {
		t.Fatalf("small baseline must fit: %v", err)
	}
	compactor.RemoteFacts = func(context.Context, db.Run) (map[string]any, error) {
		return map[string]any{"bounded_remote_evidence": strings.Repeat("r", 30000)}, nil
	}
	if fits, err := compactor.currentContextFits(t.Context(), f.run()); err != nil || fits {
		t.Fatalf("remote evidence was omitted from compaction sizing: %v", err)
	}
	if err := compactor.Handle(t.Context(), f.task("compaction", "hard_limit")); err != nil {
		t.Fatal(err)
	}
	if run := f.run(); run.Status != "failed" || run.ErrorCode.String != "CONTEXT_COMPACTION_FAILED" {
		t.Fatal("unshrinkable context resumed instead of terminating")
	}
	if f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1 AND queue='agent'", f.r) != 0 {
		t.Fatal("oversize context entered an infinite resume loop")
	}
	sentinel := errors.New("context storage unavailable")
	compactor.RemoteFacts = func(context.Context, db.Run) (map[string]any, error) { return nil, sentinel }
	if _, err := compactor.currentContextFits(t.Context(), f.run()); !errors.Is(err, sentinel) {
		t.Fatal("storage error was converted into a compaction decision", err)
	}
}
