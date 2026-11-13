package dashboard

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"testing"
)

func TestPostgresOperationsTemplatePreparesAndNativeSamplesWithoutTelemetry(t *testing.T) {
	f := newAnalysisFixture(t)
	cluster := uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Native overview','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", []any{cluster, f.actor.EnterpriseID}},
		{"INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", []any{uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, cluster}},
	} {
		if _, err := f.store.Pool.Exec(t.Context(), statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	inventory := &overviewInventoryFixture{snapshot: kubernetesworkbench.Snapshot{ClusterID: cluster, Complete: true, Counts: map[string]map[string]int{"pod": {"total": 0}}, Errors: map[string]string{}}}
	runtime := Runtime{Store: f.store, Inventory: inventory}
	spec := operationsOverviewTemplate().Spec
	input := ExecutionInput{ResourceMode: "explicit", ResourceIDs: []uuid.UUID{cluster}, KubernetesScope: &dashboardparams.KubernetesScope{ClusterID: cluster}}
	prepared, _, err := runtime.prepareSpec(t.Context(), f.actor, spec, input)
	if err != nil || len(prepared.Panels) != 12 {
		t.Fatalf("full built-in template cannot prepare: %+v %v", prepared, err)
	}
	spec.Panels = spec.Panels[:1]
	report, sample, err := runtime.VerifyWithScope(t.Context(), f.actor, spec, input)
	if err != nil || !report.Valid || len(sample) == 0 || inventory.calls != 1 {
		t.Fatalf("native editor sample incorrectly depends on telemetry capability: %+v %s %v calls=%d", report, sample, err, inventory.calls)
	}
}
