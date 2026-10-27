package kubernetesworkbench

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
	"testing"
)

func TestPostgresAuthorizationCatalogPagesAndNamespaceInheritance(t *testing.T) {
	f := newMutationFixture(t)
	ctx := t.Context()
	catalog := authorization.Service{Store: f.service.Store}
	for _, environment := range []string{"production", "testing"} {
		id := uuid.New()
		f.exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'orders',$4,$5)", id, f.actor.EnterpriseID, id.String(), environment, f.actor.SubjectID)
	}
	first, err := catalog.GrantCatalogPage(ctx, f.actor.EnterpriseID, f.actor.SubjectID, "user", "application", "orders", uuid.Nil, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first catalog page: %d %v", len(first), err)
	}
	second, err := catalog.GrantCatalogPage(ctx, f.actor.EnterpriseID, f.actor.SubjectID, "user", "application", "orders", first[0].ResourceID, 1)
	if err != nil || len(second) != 1 || first[0].ResourceID == second[0].ResourceID || first[0].Environment == second[0].Environment {
		t.Fatalf("application identity/environment pagination: %+v %+v %v", first, second, err)
	}
	namespaces, err := catalog.GrantCatalogPage(ctx, f.actor.EnterpriseID, f.actor.SubjectID, "user", "kubernetes_namespace", "orders-uid", uuid.Nil, 10)
	if err != nil || len(namespaces) != 1 || namespaces[0].SourceUID != "orders-uid" || !namespaces[0].Direct || namespaces[0].Inherited || namespaces[0].ParentID.UUID != f.cluster {
		t.Fatalf("namespace catalog: %+v %v", namespaces, err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, f.cluster)
	namespaces, err = catalog.GrantCatalogPage(ctx, f.actor.EnterpriseID, f.actor.SubjectID, "user", "kubernetes_namespace", "owned cluster", uuid.Nil, 10)
	if err != nil || len(namespaces) != 1 || !namespaces[0].Direct || !namespaces[0].Inherited || !slices.Contains(namespaces[0].Sources, "cluster:owned cluster") {
		t.Fatalf("direct grant concealed cluster inheritance: %+v %v", namespaces, err)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_type='kubernetes_cluster'", f.actor.EnterpriseID)
	f.exec("UPDATE kubernetes_namespace_resources SET status='deleted' WHERE id=$1", f.namespace)
	_, err = f.service.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: "orders", NamespaceUid: "replacement-uid"})
	if err != nil {
		t.Fatal(err)
	}
	namespaces, err = catalog.GrantCatalogPage(ctx, f.actor.EnterpriseID, f.actor.SubjectID, "user", "kubernetes_namespace", "", uuid.Nil, 10)
	if err != nil || len(namespaces) != 1 || namespaces[0].Direct || namespaces[0].Inherited || namespaces[0].SourceUID != "replacement-uid" {
		t.Fatalf("old namespace grant inherited by replacement: %+v %v", namespaces, err)
	}
	role, target := uuid.New(), uuid.New()
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'expired role')", role, f.actor.EnterpriseID)
	f.exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'catalog database','database','mysql','{}',$3)", target, f.actor.EnterpriseID, f.actor.SubjectID)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id,valid_until) VALUES($1,$2,'user',$3,$4,now()-interval '1 minute')", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, role)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'role',$3,'monitoring_target',$4)", uuid.New(), f.actor.EnterpriseID, role, target)
	targets, err := catalog.GrantCatalogPage(ctx, f.actor.EnterpriseID, f.actor.SubjectID, "user", "monitoring_target", "", uuid.Nil, 10)
	if err != nil || len(targets) != 1 || targets[0].Inherited {
		t.Fatalf("expired role inherited: %+v %v", targets, err)
	}
}
