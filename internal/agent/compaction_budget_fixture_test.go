package agent

import (
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Size the fixture against today's actual fixed request, not a historical
// instruction length. Production's conservative wire-byte policy is unchanged.
func (f *recoveryFixture) compactionFixtureBudget(t *testing.T) modelprovider.ContextBudget {
	t.Helper()
	run := f.run()
	loop := Loop{Store: f.store}
	messages, current, _, source, err := loop.messages(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := f.store.Queries.GetEnabledAIModelRevision(t.Context(), db.GetEnabledAIModelRevisionParams{ModelID: run.ModelID, EnterpriseID: run.EnterpriseID, Revision: run.ModelRevision})
	if err != nil {
		t.Fatal(err)
	}
	revision.ContextWindowTokens = 256 * 1024
	projection, err := loop.context(t.Context(), run, revision, messages, current, nil, source)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := (modelprovider.Provider{Protocol: modelprovider.Protocol(revision.ApiProtocol), BaseURL: revision.BaseUrl}).RequestBytes(modelprovider.Request{Model: revision.ProviderModelID, Messages: projection.Messages, MaxTokens: int(revision.MaxOutputTokens)})
	if err != nil {
		t.Fatal(err)
	}
	window := 32768
	for modelprovider.NewContextBudget(window, int(revision.MaxOutputTokens)).HardLimit <= fixed+12*1024 {
		window += 4096
		if window > 256*1024 {
			t.Fatal("fixed context exceeds the controlled compaction fixture bound")
		}
	}
	f.exec("UPDATE ai_models SET context_window_tokens=$2 WHERE id=$1", f.m, window)
	f.exec("UPDATE ai_model_revisions SET context_window_tokens=$2 WHERE model_id=$1", f.m, window)
	budget := modelprovider.NewContextBudget(window, int(revision.MaxOutputTokens))
	t.Logf("current fixed request bytes=%d fixture_window=%d hard_limit=%d; history must still independently exceed this limit", fixed, window, budget.HardLimit)
	return budget
}
