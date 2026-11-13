package dashboard

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestPostgresOverviewSelectionDefaultConflictResetAndUnavailable(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	cluster := uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Overview cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.actor.EnterpriseID)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, cluster)
	exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,p FROM role_bindings CROSS JOIN unnest(ARRAY['kubernetes.manage','kubernetes.read','pending_action.confirm']) p WHERE subject_id=$1", f.actor.SubjectID)
	key := bytes.Repeat([]byte{6}, 32)
	service := f.service
	service.Actions = resource.PendingActionService{Store: f.store, Key: key, Idempotency: postgres.Idempotency{Key: key}}
	if err := service.SyncTemplates(ctx, []TemplateDefinition{operationsOverviewTemplate()}); err != nil {
		t.Fatal(err)
	}
	selection, err := service.Overview(ctx, f.actor, cluster, 1)
	if err != nil || selection.Dashboard == nil || selection.Mode != "default" {
		t.Fatal(selection, err)
	}
	// A v2 custom published board and its existing ordinary resource binding
	// coexist with one independently versioned overview selection.
	f.publish(t, f.a, 2, operationsOverviewTemplate().Spec)
	board, _, err := service.Get(ctx, f.actor, f.a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Queries.CreateDashboardBinding(ctx, db.CreateDashboardBindingParams{ID: uuid.New(), EnterpriseID: f.actor.EnterpriseID, DashboardID: f.a, ResourceType: "kubernetes_cluster", ResourceID: cluster, CreatedBy: f.actor.SubjectID}); err != nil {
		t.Fatal(err)
	}
	input := OverviewInput{DashboardID: f.a, ExpectedDashboardVersion: board.Version, ExpectedSelectionVersion: 0, ExpectedResourceVersion: 1}
	first, err := service.PreviewOverview(ctx, f.actor, cluster, input, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.PreviewOverview(ctx, f.actor, cluster, input, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	extension := ActionExtension{}
	workflow := actionservice.Service{Store: f.store, Idempotency: service.Actions.Idempotency, Resources: resource.Service{Store: f.store, Actions: service.Actions, Extension: extension}}
	confirm := func(pending db.PendingAction) error {
		confirmed, e := workflow.Confirm(ctx, f.actor.SubjectID.String(), uuid.NewString(), f.actor.EnterpriseID, 1, false, pending.ActionRef, uuid.NewString())
		if e != nil {
			return e
		}
		return f.store.InTx(ctx, func(q *db.Queries) error {
			_, e := service.Actions.ExecuteReady(ctx, q, confirmed.PendingAction, extension.RevalidateAction, extension.CommitAction)
			return e
		})
	}
	if err = confirm(first); err != nil {
		t.Fatal(err)
	}
	ordinary, e := f.store.Queries.ListResourceDashboardBindings(ctx, db.ListResourceDashboardBindingsParams{EnterpriseID: f.actor.EnterpriseID, ResourceType: "kubernetes_cluster", ResourceID: cluster})
	if e != nil || len(ordinary) != 1 {
		t.Fatal("overview displaced ordinary binding", ordinary, e)
	}
	if err = confirm(second); err == nil {
		t.Fatal("stale selection was replayed")
	}
	selection, err = service.Overview(ctx, f.actor, cluster, 1)
	if err != nil || selection.Mode != "custom" || selection.SelectionVersion != 1 || selection.Dashboard.ID != f.a {
		t.Fatal(selection, err)
	}
	exec("UPDATE dashboards SET lifecycle='archived' WHERE id=$1", f.a)
	selection, err = service.Overview(ctx, f.actor, cluster, 1)
	if err != nil || selection.Dashboard != nil || selection.UnavailableCode == "" {
		t.Fatal("invalid selected dashboard silently fell back", selection, err)
	}
	reset := OverviewInput{DashboardID: selection.DefaultDashboard.ID, ExpectedDashboardVersion: selection.DefaultDashboard.Version, ExpectedSelectionVersion: 1, ExpectedResourceVersion: 1, Reset: true}
	pending, err := service.PreviewOverview(ctx, f.actor, cluster, reset, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if err = confirm(pending); err != nil {
		t.Fatal(err)
	}
	selection, err = service.Overview(ctx, f.actor, cluster, 1)
	if err != nil || selection.Mode != "default" || selection.SelectionVersion != 2 {
		t.Fatal(selection, err)
	}
	if _, err = service.PreviewOverview(ctx, f.actor, cluster, reset, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatal("reset ABA version accepted", err)
	}
	_, err = service.OverviewCandidates(context.Background(), f.actor, cluster, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
}
