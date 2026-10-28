package monitoring

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresCollectionInventoryExcludesRetiredOwnersBeforePaging(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	oldCollector := f.collector()
	old, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: oldCollector})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE hosts SET name='Retired owned host' WHERE id=$1", old.ResourceID)
	liveCollector := f.collector()
	live, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: liveCollector})
	if err != nil {
		t.Fatal(err)
	}
	prefix := uuid.New()
	ids := []uuid.UUID{prefix, prefix, prefix}
	for i := range ids {
		ids[i][15] = byte(i)
	}
	for i, key := range []string{"host_metrics", "host_metrics", "collector_metrics"} {
		collector, host := liveCollector, live.ResourceID
		if i == 0 {
			collector, host = oldCollector, old.ResourceID
		}
		f.exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,desired_enabled,created_by) VALUES($1,$2,'Owned inventory',$3,'host',$4,$5,$6,$7)", ids[i], f.a.EnterpriseID, key, host, collector, i == 2, f.a.SubjectID)
	}
	f.exec("UPDATE hosts SET status='deleted' WHERE id=$1", old.ResourceID)
	first, err := f.s.Collections(ctx, f.a, ListInput{Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != ids[1] || first.Items[0].DesiredEnabled {
		t.Fatal("retired owner consumed the first page or intentionally stopped collection disappeared", first, err)
	}
	if first.NextCursor == "" {
		t.Fatal("live second page missing")
	}
	second, err := f.s.Collections(ctx, f.a, ListInput{Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != ids[2] {
		t.Fatal("paging skipped a current collection", second, err)
	}
	// Retirement affects the current directory, not the retained record itself.
	retained, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: ids[0]})
	if err != nil || retained.ID != ids[0] {
		t.Fatal("historical collection was removed", err)
	}
}
