package agent

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestRunModeGuidanceUsesSnapshotInActualModelRequest(t *testing.T) {
	f := newRecoveryFixture(t)
	loop := Loop{Store: f.store}
	check := func(run db.Run, mode string) {
		t.Helper()
		messages, input, _, source, err := loop.messages(t.Context(), run)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := loop.context(t.Context(), run, db.AiModelRevision{ContextWindowTokens: 32768, MaxOutputTokens: 1024}, messages, input, nil, source)
		if err != nil {
			t.Fatal(err)
		}
		system := projection.Messages[0]
		if system.Role != "system" || !strings.Contains(system.Content, "Current Run mode: "+mode+".") {
			t.Fatal("current run mode is missing from provider system instructions")
		}
		if !strings.Contains(system.Content, toolruntime.EvidenceScopeInstructions) {
			t.Fatal("evidence scope constraints did not reach the actual provider system message")
		}
		if !strings.Contains(system.Content, toolruntime.RemoteCommandStyleInstructions) {
			t.Fatal("concise commands and native failure handling did not reach the actual system request")
		}
		if !strings.Contains(system.Content, "records counts the top-level time-series entries") || !strings.Contains(system.Content, "samples counts timestamp-value points") {
			t.Fatal("metric record and sample semantics did not reach the actual model system message")
		}
		if mode == "plan" && (!strings.Contains(system.Content, `tool.search({"category":"workflow"})`) || !strings.Contains(system.Content, "Do not package a requested repair into connect")) {
			t.Fatal("plan instructions lost their discovery path or repair boundary")
		}
	}
	f.exec("UPDATE conversations SET mode='plan' WHERE id=$1", f.c)
	check(f.run(), "execute")
	f.exec("UPDATE runs SET status='succeeded' WHERE id=$1", f.r)
	next, err := f.store.Queries.CreateRun(t.Context(), db.CreateRunParams{ID: uuid.New(), ConversationID: f.c, EnterpriseID: f.e, ActorUserID: f.u, ModelID: f.m, ModelRevision: 1, Locale: "zh-CN", AuthorizationVersion: 1, Checkpoint: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE conversations SET mode='execute' WHERE id=$1", f.c)
	check(next, "plan")
}
