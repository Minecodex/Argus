package dashboard

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresResourceDashboardBindings(t *testing.T) {
	url := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ARGUS_DASHBOARD_TEST_DATABASE_URL required")
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, editorRole, managerRole, user, manager, host, cluster := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Binding test',$2,'UTC')`, enterprise, "binding-"+enterprise.String())
	exec(`INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Bindings')`, department, enterprise)
	for _, id := range []uuid.UUID{user, manager} {
		exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Binding user')`, id, enterprise, department, id.String())
	}
	for _, role := range []uuid.UUID{editorRole, managerRole} {
		exec(`INSERT INTO roles(id,enterprise_id,name,description,builtin) VALUES($1,$2,$3,'',false)`, role, enterprise, role.String())
		exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read'),($1,'host.read'),($1,'host.manage'),($1,'kubernetes.read'),($1,'kubernetes.manage')`, role)
	}
	exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.manage')`, editorRole)
	exec(`INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4),($5,$2,'user',$6,$7)`, uuid.New(), enterprise, user, editorRole, uuid.New(), manager, managerRole)
	exec(`INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Binding host','127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))`, host, enterprise)
	exec(`INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Binding cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))`, cluster, enterprise)
	grant := func(subject uuid.UUID, kind string, id uuid.UUID) {
		exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,$4,$5) ON CONFLICT(enterprise_id,subject_type,subject_id,resource_type,resource_id) DO UPDATE SET status='active'`, uuid.New(), enterprise, subject, kind, id)
	}
	for _, subject := range []uuid.UUID{user, manager} {
		grant(subject, "host", host)
		grant(subject, "kubernetes_cluster", cluster)
	}
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", nullID(enterprise)); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{9}, 32)
	service := Service{Store: store, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}
	actor := Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	managerActor := actor
	managerActor.SubjectID = manager
	extension := ActionExtension{}
	workflow := actionservice.Service{Store: store, Idempotency: postgres.Idempotency{Key: key}, Resources: resource.Service{Store: store, Actions: service.Actions, Extension: extension}}
	confirmationKeys := map[string]string{}
	confirm := func(action db.PendingAction) (resource.ActionCommitResult, error) {
		t.Helper()
		var result resource.ActionCommitResult
		confirmationKey := uuid.NewString()
		confirmationKeys[action.ActionRef] = confirmationKey
		confirmed, err := workflow.Confirm(ctx, action.CreatorSubjectID.String(), uuid.NewString(), enterprise, action.AuthorizationVersion, false, action.ActionRef, confirmationKey)
		if err != nil {
			return result, err
		}
		err = store.InTx(ctx, func(q *db.Queries) error {
			var e error
			result, e = service.Actions.ExecuteReady(ctx, q, confirmed.PendingAction, extension.RevalidateAction, extension.CommitAction)
			return e
		})
		return result, err
	}
	draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Service dashboard"})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	published, err := confirm(publication.Action)
	if err != nil {
		t.Fatal(err)
	}
	dashboardID := published.ResourceID
	grant(manager, "dashboard", dashboardID)
	inputFor := func(who Actor, kind string, id uuid.UUID) BindingInput {
		t.Helper()
		item, _, e := service.Get(ctx, who, dashboardID)
		if e != nil {
			t.Fatal(e)
		}
		scope, e := service.ResourceBindings(ctx, who, kind, id)
		if e != nil {
			t.Fatal(e)
		}
		return BindingInput{Operation: "attach", DashboardID: dashboardID, ExpectedDashboardVersion: item.Version, ExpectedResourceVersion: scope.Version}
	}
	preview := func(who Actor, kind string, id uuid.UUID, input BindingInput) db.PendingAction {
		t.Helper()
		action, e := service.PreviewBinding(ctx, who, kind, id, input, uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		return action
	}
	count := func() int {
		t.Helper()
		var value int
		if e := store.Pool.QueryRow(ctx, `SELECT count(*) FROM dashboard_bindings WHERE enterprise_id=$1`, enterprise).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}

	// Resource managers need dashboard read access, not dashboard edit permission.
	hostInput := inputFor(managerActor, "host", host)
	first := preview(managerActor, "host", host, hostInput)
	second := preview(managerActor, "host", host, hostInput)
	if count() != 0 {
		t.Fatal("preview mutated bindings")
	}
	created, err := confirm(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = confirm(second); err == nil {
		t.Fatal("concurrent attach silently reused an unseen binding")
	}
	if _, err = workflow.Confirm(ctx, first.CreatorSubjectID.String(), uuid.NewString(), enterprise, first.AuthorizationVersion, false, first.ActionRef, confirmationKeys[first.ActionRef]); err != nil {
		t.Fatalf("repeat confirmation not idempotent: %v", err)
	}
	if count() != 1 {
		t.Fatal("duplicate attach")
	}
	clustered, err := confirm(preview(managerActor, "kubernetes_cluster", cluster, inputFor(managerActor, "kubernetes_cluster", cluster)))
	if err != nil || clustered.ResourceID == created.ResourceID {
		t.Fatalf("cluster attach: %v", err)
	}
	visible, err := service.DashboardBindings(ctx, managerActor, dashboardID)
	if err != nil || len(visible) != 2 {
		t.Fatalf("summary scope: %v %v", visible, err)
	}

	// Revoking one subject only hides their entry. Other readers retain access.
	exec(`UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2 AND resource_type='dashboard'`, enterprise, manager)
	hidden, err := service.ResourceBindings(ctx, managerActor, "host", host)
	if err != nil || len(hidden.Items) != 0 {
		t.Fatalf("revoked dashboard leaked: %v %v", hidden, err)
	}
	own, err := service.ResourceBindings(ctx, actor, "host", host)
	if err != nil || len(own.Items) != 1 || count() != 2 {
		t.Fatal("subject revocation mutated global shortcuts")
	}
	grant(manager, "dashboard", dashboardID)
	detach := inputFor(managerActor, "host", host)
	detach.Operation = "detach"
	detach.BindingID = own.Items[0].Binding.ID
	detach.ExpectedBindingVersion = own.Items[0].Binding.Version
	staleDetach := preview(managerActor, "host", host, detach)
	detached, err := confirm(preview(managerActor, "host", host, detach))
	if err != nil || detached.ResourceID != created.ResourceID {
		t.Fatalf("detach: %v", err)
	}
	reattached, err := confirm(preview(managerActor, "host", host, inputFor(managerActor, "host", host)))
	if err != nil || reattached.ResourceID == created.ResourceID {
		t.Fatalf("re-attach identity: %v", err)
	}
	if _, err = confirm(staleDetach); err == nil {
		t.Fatal("old detach removed a new attachment")
	}

	// Resource changes, lifecycle changes, and fresh authorization all invalidate old previews.
	current, _ := service.ResourceBindings(ctx, managerActor, "host", host)
	detach = inputFor(managerActor, "host", host)
	detach.Operation = "detach"
	detach.BindingID = current.Items[0].Binding.ID
	detach.ExpectedBindingVersion = current.Items[0].Binding.Version
	staleVersion := preview(managerActor, "host", host, detach)
	exec(`UPDATE hosts SET resource_version=resource_version+1 WHERE id=$1`, host)
	if _, err = confirm(staleVersion); err == nil {
		t.Fatal("resource version drift was ignored")
	}
	detach.ExpectedResourceVersion++
	staleGrant := preview(managerActor, "host", host, detach)
	exec(`UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2 AND resource_type='host'`, enterprise, manager)
	if _, err = confirm(staleGrant); err == nil {
		t.Fatal("revoked resource grant allowed commit")
	}
	grant(manager, "host", host)
	if count() != 2 {
		t.Fatal("failed confirmation changed bindings")
	}
	item, _, _ := service.Get(ctx, actor, dashboardID)
	archive, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "archive", ID: dashboardID, ExpectedVersion: item.Version}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	staleDashboard := preview(managerActor, "host", host, detach)
	archived, err := confirm(archive)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err = service.ResourceBindings(ctx, actor, "host", host)
	if err != nil || len(hidden.Items) != 0 || count() != 2 {
		t.Fatal("archive should hide and retain bindings")
	}
	restore, err := service.PreviewLifecycle(ctx, actor, LifecycleInput{Operation: "restore", ID: dashboardID, ExpectedVersion: archived.ResourceVersion}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = confirm(restore); err != nil {
		t.Fatal(err)
	}
	if _, err = confirm(staleDashboard); err == nil {
		t.Fatal("archive and restore preserved old preview")
	}
	own, err = service.ResourceBindings(ctx, managerActor, "host", host)
	if err != nil || len(own.Items) != 1 {
		t.Fatal("restored binding unavailable")
	}
	if _, err = service.ResourceBindings(ctx, managerActor, "namespace", cluster); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unexpected resource kind accepted: %v", err)
	}
	foreign := managerActor
	foreign.EnterpriseID = uuid.New()
	if _, err = service.ResourceBindings(ctx, foreign, "host", host); !errors.Is(err, ErrDenied) {
		t.Fatalf("cross-enterprise read: %v", err)
	}
	detach.ExpectedDashboardVersion = own.Items[0].Dashboard.Version
	exec(`DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='host.manage'`, managerRole)
	if _, err = service.PreviewBinding(ctx, managerActor, "host", host, detach, uuid.NewString()); !errors.Is(err, ErrDenied) {
		t.Fatalf("resource read permitted mutation: %v", err)
	}
	exec(`UPDATE hosts SET status='deleted',resource_version=resource_version+1 WHERE id=$1`, host)
	if _, err = service.ResourceBindings(ctx, actor, "host", host); err == nil {
		t.Fatal("deleted resource shortcut remained usable")
	}
	summary, err := service.DashboardBindings(ctx, actor, dashboardID)
	if err != nil || len(summary) != 1 || summary[0].Binding.ResourceType != "kubernetes_cluster" {
		t.Fatalf("deleted host leaked in summary: %v %v", summary, err)
	}

	// Exercise the shared grant service, including version invalidation and role inheritance.
	viewer := uuid.New()
	exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Viewer')`, viewer, enterprise, department, viewer.String())
	exec(`INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)`, uuid.New(), enterprise, viewer, managerRole)
	authz := authorization.Service{Store: store}
	viewerActor := Actor{EnterpriseID: enterprise, SubjectID: viewer, SubjectType: "user", AuthorizationVersion: 1}
	if _, _, err := service.Get(ctx, viewerActor, dashboardID); !errors.Is(err, ErrDenied) {
		t.Fatalf("ungranted dashboard accessible: %v", err)
	}
	batch := authorization.GrantBatchInput{SubjectType: "user", SubjectID: viewer, ResourceType: "dashboard", ResourceIDs: []uuid.UUID{dashboardID}, ExpectedVersion: 1}
	if err := authz.UpdateGrantBatch(ctx, user.String(), enterprise, batch); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Get(ctx, viewerActor, dashboardID); !errors.Is(err, ErrDenied) {
		t.Fatal("stale authorization version was accepted")
	}
	viewerActor.AuthorizationVersion = 2
	if _, _, err := service.Get(ctx, viewerActor, dashboardID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResourceBindings(ctx, viewerActor, "kubernetes_cluster", cluster); !errors.Is(err, ErrDenied) {
		t.Fatal("dashboard grant implied resource access")
	}
	resourceBatch := authorization.GrantBatchInput{SubjectType: "user", SubjectID: viewer, ResourceType: "kubernetes_cluster", ResourceIDs: []uuid.UUID{cluster}, ExpectedVersion: 2}
	if err := authz.UpdateGrantBatch(ctx, user.String(), enterprise, resourceBatch); err != nil {
		t.Fatal(err)
	}
	viewerActor.AuthorizationVersion = 3
	if available, err := service.ResourceBindings(ctx, viewerActor, "kubernetes_cluster", cluster); err != nil || len(available.Items) != 1 {
		t.Fatalf("authorized shortcut unavailable: %v %v", available, err)
	}
	batch.Remove = true
	batch.ExpectedVersion = 3
	if err := authz.UpdateGrantBatch(ctx, user.String(), enterprise, batch); err != nil {
		t.Fatal(err)
	}
	viewerActor.AuthorizationVersion = 4
	if _, _, err := service.Get(ctx, viewerActor, dashboardID); !errors.Is(err, ErrDenied) {
		t.Fatal("removed dashboard grant remained usable")
	}
	roleVersion, err := authz.CurrentAuthorizationVersion(ctx, enterprise, managerRole, "role")
	if err != nil {
		t.Fatal(err)
	}
	if err := authz.UpdateGrantBatch(ctx, user.String(), enterprise, authorization.GrantBatchInput{SubjectType: "role", SubjectID: managerRole, ResourceType: "dashboard", ResourceIDs: []uuid.UUID{dashboardID}, ExpectedVersion: roleVersion}); err != nil {
		t.Fatal(err)
	}
	viewerActor.AuthorizationVersion, err = authz.CurrentAuthorizationVersion(ctx, enterprise, viewer, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Get(ctx, viewerActor, dashboardID); err != nil {
		t.Fatalf("inherited dashboard grant unavailable: %v", err)
	}
	batch.ExpectedVersion = viewerActor.AuthorizationVersion
	if err := authz.UpdateGrantBatch(ctx, user.String(), enterprise, batch); !errors.Is(err, authorization.ErrInheritedGrant) {
		t.Fatalf("inherited grant could be removed at user: %v", err)
	}
	catalog, err := authz.ListGrantResources(ctx, enterprise, viewer, "user", "dashboard")
	if err != nil || len(catalog) != 1 || !catalog[0].Inherited || catalog[0].Direct {
		t.Fatalf("dashboard catalog inheritance: %v %v", catalog, err)
	}
	batch.SubjectID = uuid.New()
	batch.ExpectedVersion = 0
	batch.Remove = false
	if err := authz.UpdateGrantBatch(ctx, user.String(), enterprise, batch); err == nil {
		t.Fatal("foreign or missing subject accepted")
	}
}
