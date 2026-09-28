package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestDashboardAuditProjectionKeepsReferencesButNoResultsOrVariableValues(t *testing.T) {
	board, revision, source := uuid.New(), uuid.New(), uuid.New()
	at := time.Now().UTC()
	result := Execution{DashboardID: board, RevisionID: revision, ExecutionID: uuid.New(), ContextToken: "private-context-token", From: at, To: at.Add(time.Minute), Variables: map[string]Selection{"env": {Values: []string{"private-variable"}}}, Panels: []PanelExecution{{ID: "logs", Status: "partial", Sources: []ResolvedSource{{ID: source, Revision: 4, Type: "otlp"}}, Targets: []TargetExecution{{ID: "main", Status: "partial", QueryHash: "query-digest", Data: map[string]any{"body": "private-log-body"}, Meta: queryengine.QueryMeta{ScannedRows: 17, ReturnedRows: 2, CacheHit: true}}}}}}
	details := executionAuditDetails(result, ExecutionInput{})
	raw, _ := json.Marshal(details)
	for _, secret := range []string{"private-context-token", "private-variable", "private-log-body"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("audit leaked %s", secret)
		}
	}
	for _, reference := range []string{board.String(), revision.String(), source.String(), "query-digest", `"returned_rows":2`, `"cache_hit":true`} {
		if !strings.Contains(string(raw), reference) {
			t.Fatalf("missing %s in %s", reference, raw)
		}
	}
	before := details["parameters_hash"]
	result.Variables["env"] = Selection{Values: []string{"different-variable"}}
	if executionAuditDetails(result, ExecutionInput{})["parameters_hash"] == before {
		t.Fatal("condition changes lost audit identity")
	}
}

func TestDraftAuditRecordsPublishedAndBaseRevisions(t *testing.T) {
	draft := db.DashboardDraft{ID: uuid.New(), DashboardID: nullID(uuid.New()), BaseRevisionID: nullID(uuid.New()), PublishedRevisionID: nullID(uuid.New()), DraftVersion: 9, BaseObjectVersion: 3, Spec: []byte(`{}`), ProposedBindings: []byte(`[]`)}
	fields := draftAuditDetails(draft)
	if fields["draft_version"] != int64(9) || fields["revision_id"] != draft.PublishedRevisionID.UUID || fields["base_revision_id"] != draft.BaseRevisionID.UUID {
		t.Fatal("audit cannot distinguish editing baseline from publication")
	}
}
