package dashboard

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"testing"
)

func TestPostgresExplicitNamespaceScopeDoesNotExpandToSiblingNamespaces(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	cluster, other := uuid.New(), uuid.New()
	ns, sibling, foreign := uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{cluster, other} {
		exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,$3,'https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", id, f.actor.EnterpriseID, id.String())
	}
	for _, row := range []struct{ id, cluster uuid.UUID }{{ns, cluster}, {sibling, cluster}, {foreign, other}} {
		exec("INSERT INTO kubernetes_namespace_resources(id,enterprise_id,cluster_id,name,namespace_uid,resource_version) VALUES($1,$2,$3,$4,$4,'1')", row.id, f.actor.EnterpriseID, row.cluster, row.id.String())
	}
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, ns)
	r := Runtime{Store: f.store}
	input := ExecutionInput{ResourceMode: "explicit", ResourceIDs: []uuid.UUID{cluster}, KubernetesScope: &dashboardparams.KubernetesScope{ClusterID: cluster, NamespaceIDs: []uuid.UUID{ns}}}
	normalized, err := r.normalizeKubernetesScope(ctx, f.actor, input)
	if err != nil || len(normalized.ResourceIDs) != 1 || normalized.ResourceIDs[0] != ns {
		t.Fatal("selected namespace widened to siblings", normalized, err)
	}
	spec := operationsOverviewTemplate().Spec
	spec.Panels = spec.Panels[:1]
	prepared, _, err := r.prepareSpec(ctx, f.actor, spec, input)
	if err != nil || len(prepared.Resources) != 1 || prepared.Resources[0].ID != ns {
		t.Fatal(prepared, err)
	}
	input.KubernetesScope.NamespaceIDs = []uuid.UUID{sibling}
	if _, _, err = r.prepareSpec(ctx, f.actor, spec, input); !errors.Is(err, ErrDenied) {
		t.Fatal("ungranted sibling accepted", err)
	}
	input.KubernetesScope.NamespaceIDs = []uuid.UUID{foreign}
	if _, err = r.normalizeKubernetesScope(ctx, f.actor, input); !errors.Is(err, ErrDenied) {
		t.Fatal("namespace from another cluster accepted", err)
	}
}
