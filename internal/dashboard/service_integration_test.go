package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresDraftPublicationIsolation(t *testing.T) {
	url := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ARGUS_DASHBOARD_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	if err := postgres.RunMigrations(ctx, url, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	enterprise, department, role, user, other := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Dashboard test',$2,'UTC')`, enterprise, "dashboard-"+enterprise.String())
	exec(`INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Test')`, department, enterprise)
	for _, id := range []uuid.UUID{user, other} {
		exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Editor')`, id, enterprise, department, "editor-"+id.String())
	}
	exec(`INSERT INTO roles(id,enterprise_id,identity_key,name,description,builtin) VALUES($1,$2,NULL,'Dashboard editor','',false)`, role, enterprise)
	exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read'),($1,'telemetry.dashboard.manage')`, role)
	exec(`INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'department',$3,$4)`, uuid.New(), enterprise, department, role)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", nullID(enterprise)); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{7}, 32)
	service := Service{Store: store, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}
	actor := Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	otherActor := actor
	otherActor.SubjectID = other
	extension := ActionExtension{}
	workflow := actionservice.Service{Store: store, Idempotency: postgres.Idempotency{Key: key}, Resources: resource.Service{Store: store, Actions: service.Actions, Extension: extension}}
	execute := func(preview PublicationPreview) (resource.ActionCommitResult, error) {
		var result resource.ActionCommitResult
		confirmed, err := workflow.Confirm(ctx, preview.Action.CreatorSubjectID.String(), uuid.NewString(), enterprise, preview.Action.AuthorizationVersion, false, preview.Action.ActionRef, uuid.NewString())
		if err != nil {
			return result, err
		}
		err = store.InTx(ctx, func(q *db.Queries) error {
			action := confirmed.PendingAction
			var err error
			result, err = service.Actions.ExecuteReady(ctx, q, action, extension.RevalidateAction, extension.CommitAction)
			return err
		})
		return result, err
	}

	draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Payment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Draft(ctx, otherActor, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("personal draft leaked: %v", err)
	}
	if items, err := service.List(ctx, actor); err != nil || len(items) != 0 {
		t.Fatalf("draft became published: %v %v", items, err)
	}
	first, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	created, err := execute(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(duplicate); err == nil {
		t.Fatal("two previews published one new draft twice")
	}
	item, revision, err := service.Get(ctx, actor, created.ResourceID)
	if err != nil || revision.RevisionNumber != 1 {
		t.Fatalf("publication not visible to creator: %v %v", revision, err)
	}
	if _, _, err := service.Get(ctx, otherActor, item.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("object grant bypass: %v", err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE dashboard_revisions SET name='mutated' WHERE id=$1`, revision.ID); err == nil {
		t.Fatal("published revision was mutable")
	}
	exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'dashboard',$4)`, uuid.New(), enterprise, other, item.ID)

	draft1, err := service.CreateDraft(ctx, actor, DraftInput{DashboardID: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	draft2, err := service.CreateDraft(ctx, otherActor, DraftInput{DashboardID: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := service.PreviewPublish(ctx, otherActor, draft2.ID, draft2.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	input := DraftInput{Name: "Payment R2", Spec: draft1.Spec, ExpectedVersion: draft1.DraftVersion}
	saved, err := service.SaveDraft(ctx, actor, draft1.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveDraft(ctx, actor, draft1.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale browser save overwrote draft: %v", err)
	}
	preview, err := service.PreviewPublish(ctx, actor, saved.ID, saved.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(preview); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("later publisher lost the conflict needed for reconciliation: %v", err)
	}
	item, revision, err = service.Get(ctx, actor, item.ID)
	if err != nil || revision.RevisionNumber != 2 {
		t.Fatal(err)
	}
	if _, err := service.RebaseDraft(ctx, otherActor, draft2.ID, draft2.DraftVersion, item.Version, revision.ID); err != nil {
		t.Fatal(err)
	}
	retained, err := service.Draft(ctx, otherActor, draft2.ID)
	if err != nil || retained.Name != draft2.Name {
		t.Fatalf("rebase replaced personal work: %v", err)
	}
	preview, err = service.PreviewPublish(ctx, otherActor, retained.ID, retained.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	archive, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "archive", ID: item.ID, ExpectedVersion: item.Version}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	archived, err := execute(PublicationPreview{Action: archive})
	if err != nil {
		t.Fatal(err)
	}
	restore, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "restore", ID: item.ID, ExpectedVersion: archived.ResourceVersion}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(PublicationPreview{Action: restore}); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(preview); err == nil {
		t.Fatal("archive/restore kept old preview valid")
	}
	exec(`UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2 AND resource_type='dashboard'`, enterprise, other)
	if _, err := service.Draft(ctx, otherActor, retained.ID); !errors.Is(err, ErrDenied) {
		t.Fatalf("revoked editor retained access: %v", err)
	}
	var count int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM dashboard_drafts WHERE id=$1`, retained.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("revocation deleted draft: %v", err)
	}

	// Editing after preview invalidates the old frozen publication.
	newDraft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Before"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.PreviewPublish(ctx, actor, newDraft.ID, newDraft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveDraft(ctx, actor, newDraft.ID, DraftInput{Name: "After", Spec: json.RawMessage(newDraft.Spec), ExpectedVersion: newDraft.DraftVersion})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(before); err == nil {
		t.Fatal("changed draft accepted old preview")
	}

	folderKey := uuid.NewString()
	folderPreview, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "folder.create", Name: "Services"}, folderKey)
	if err != nil {
		t.Fatal(err)
	}
	folderRetry, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "folder.create", Name: "Services"}, folderKey)
	if err != nil || folderRetry.ID != folderPreview.ID {
		t.Fatalf("folder preview retry was not idempotent: %v", err)
	}
	folder, err := execute(PublicationPreview{Action: folderPreview})
	if err != nil {
		t.Fatal(err)
	}
	folderDraft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Folder test", FolderID: folder.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := service.PreviewPublish(ctx, actor, folderDraft.ID, folderDraft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	archiveFolder, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "folder.archive", ID: folder.ResourceID, ExpectedVersion: folder.ResourceVersion}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	archivedFolder, err := execute(PublicationPreview{Action: archiveFolder})
	if err != nil {
		t.Fatal(err)
	}
	restoreFolder, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "folder.restore", ID: folder.ResourceID, ExpectedVersion: archivedFolder.ResourceVersion}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	restoredFolder, err := execute(PublicationPreview{Action: restoreFolder})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(frozen); err == nil {
		t.Fatal("folder archive/restore preserved an old publication preview")
	}
	refreshed, err := service.PreviewPublish(ctx, actor, folderDraft.ID, folderDraft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(refreshed); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "folder.archive", ID: folder.ResourceID, ExpectedVersion: restoredFolder.ResourceVersion}, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatalf("nonempty folder archived: %v", err)
	}
}
