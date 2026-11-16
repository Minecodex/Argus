package workspace

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestContextFileReferencesFollowLedgerBoundaryAndCurrentSources(t *testing.T) {
	f := newResultsFixture(t)
	f.service.Config.Enabled = true
	ctx := t.Context()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("real file bytes")))
	file, err := f.service.recordSavedFile(ctx, f.principal, f.access(), "/workspace/结果 full-uuid.json", 15, hash)
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.service.Store.Queries.GetConversation(ctx, db.GetConversationParams{ID: f.principal.ConversationID, EnterpriseID: f.principal.EnterpriseID, OwnerUserID: f.principal.UserID})
	if err != nil {
		t.Fatal(err)
	}
	request := toolruntime.ContextRequest{Principal: f.principal, ThroughSequence: c.EventSequence}
	facts, err := f.service.ContextFacts(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	index := facts["workspace_references"].(map[string]any)
	items := index["items"].([]map[string]any)
	if len(items) != 1 || items[0]["path"] != "/workspace/结果 full-uuid.json" || items[0]["file_id"] != file.ID.String() {
		t.Fatal("exact reference missing", items)
	}
	request.ThroughSequence--
	prior, err := f.service.ContextFacts(ctx, request)
	if err != nil || len(prior["workspace_references"].(map[string]any)["items"].([]map[string]any)) != 0 {
		t.Fatal("future registration leaked across ledger boundary", err)
	}
	if _, err = f.service.Store.Pool.Exec(ctx, "UPDATE workspaces SET source_result_refs=$2 WHERE id=$1", f.workspace.ID, []string{"remote:fixture"}); err != nil {
		t.Fatal(err)
	}
	f.allowed = false
	request.ThroughSequence++
	denied, err := f.service.ContextFacts(ctx, request)
	if err != nil || denied["workspace_references"].(map[string]any)["available"] != false {
		t.Fatal("revoked source retained references", denied, err)
	}
}

func TestSavedWorkingCopyGetsNewIdentityWithoutRebindingOldFile(t *testing.T) {
	f := newResultsFixture(t)
	ctx := t.Context()
	save := func(hash string) db.WorkspaceFile {
		t.Helper()
		var file db.WorkspaceFile
		err := f.service.Store.InTx(ctx, func(q *db.Queries) error {
			var err error
			file, err = registerSavedFile(ctx, q, f.principal, f.workspace, "report.md", 1, hash, "")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	original := save(fmt.Sprintf("%x", sha256.Sum256([]byte("a"))))
	same := save(original.ContentHash)
	if same.ID != original.ID {
		t.Fatal("unchanged saved bytes acquired a duplicate identity")
	}
	changed := save(fmt.Sprintf("%x", sha256.Sum256([]byte("b"))))
	if changed.ID == original.ID {
		t.Fatal("changed content silently rebound the old file ID")
	}
	if _, err := f.service.Store.Queries.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: original.ID, EnterpriseID: f.principal.EnterpriseID}); err != pgx.ErrNoRows {
		t.Fatal("old working-copy identity remains usable", err)
	}
	if changed.MediaType != "text/markdown" {
		t.Fatal("saved file lost its MIME", changed.MediaType)
	}
}
