package monitoring

import (
	"encoding/json"
	"errors"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestPostgresHostFilesKeepContainerLogsInKubernetesEntry(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'telemetry.collector.manage' FROM role_bindings WHERE enterprise_id=$1", f.a.EnterpriseID)
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['filelog','file_storage'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	for i, path := range []string{"/var/log/pods/orders_abc/main/0.log", "/var/log/containers/orders_default_main-abc.log"} {
		raw, _ := json.Marshal(map[string]any{"scope": "system", "include": []string{path}})
		_, e := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: []string{"reserved pod path", "reserved container symlink"}[i], Integration: "filelog", CollectorID: collector, Enabled: true, Config: raw}})
		if !errors.Is(e, ErrInvalid) {
			t.Fatal("host rule accepted Kubernetes-owned paths", path, e)
		}
	}
	var count int
	if e := f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_collections WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&count); e != nil || count != 0 {
		t.Fatal("invalid container paths left a collection", count, e)
	}
	input := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "ordinary host system directory", Integration: "filelog", CollectorID: collector, Enabled: true, Config: json.RawMessage(`{"scope":"system","include":["/var/log/ordinary/*.log"]}`)}}
	created, e := f.apply(input)
	if e != nil {
		t.Fatal("ordinary host rule failed", e)
	}
	row, e := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if e != nil || row.TargetType != "host" {
		t.Fatal("ordinary host ownership changed", e)
	}
}
