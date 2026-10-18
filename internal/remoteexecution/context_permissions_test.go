package remoteexecution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestReadPermissionPreservesOwnFactsWithoutGrantingExecution(t *testing.T) {
	f := newFixture(t)
	view, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("inspect"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='remote_execution.execute'", f.role)
	run, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: f.r, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := f.service.ModelFacts(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(facts)
	if !strings.Contains(string(raw), view.ID.String()) {
		t.Fatal("readable own execution disappeared when execute permission was removed")
	}
	if _, err = f.service.Prepare(t.Context(), f.call(), "connect", f.input("new command")); !errorsIsCode(err, "REMOTE_EXECUTION_FORBIDDEN") {
		t.Fatal("read permission granted a new execution", err)
	}
	f.exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='remote_execution.read'", f.role)
	facts, err = f.service.ModelFacts(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(facts)
	if strings.Contains(string(raw), view.ID.String()) {
		t.Fatal("revoked read permission leaked execution context")
	}
}
