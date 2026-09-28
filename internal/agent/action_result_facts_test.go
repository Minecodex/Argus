package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
)

func TestPublicActionFactsDistinguishCreatedIdentityAndVersionCounters(t *testing.T) {
	f := newRecoveryFixture(t)
	action, _, _ := terminalActionFixture(t, f)
	created := uuid.New()
	f.exec("UPDATE pending_actions SET status='succeeded',result_resource_type='dashboard',result_resource_id=$2,result_resource_version=2,result_summary='Published dashboard revision 1' WHERE id=$1", action.ID, created)
	facts, _, err := conversation.ContextFacts(t.Context(), f.store.Queries, f.e, f.u, f.c)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(facts)
	var value struct {
		Actions []struct {
			Input   *string `json:"resource_id"`
			Result  string  `json:"result_resource_id"`
			Version int     `json:"result_resource_version"`
		} `json:"actions"`
	}
	if json.Unmarshal(raw, &value) != nil || len(value.Actions) != 1 || value.Actions[0].Input != nil || value.Actions[0].Result != created.String() || value.Actions[0].Version != 2 {
		t.Fatalf("missing authoritative creation outcome: %s", raw)
	}
	text, err := (Loop{Store: f.store}).executionVerification(t.Context(), f.run())
	if err != nil || !strings.Contains(text, created.String()) || !strings.Contains(text, "not dashboard revision") {
		t.Fatalf("ambiguous verification facts: %v", err)
	}
	if strings.Contains(string(raw), "immutable_plan") || strings.Contains(string(raw), "commit_token") {
		t.Fatal("private confirmation data exposed")
	}
}
