package monitoring

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresOwnBuiltinCollectionsAreStableAndIndependent(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collectorID := f.collector()
	collector, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collectorID})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['hostmetrics','prometheus','nop'] WHERE id=$1", collector.DistributionVersionID)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role.id,permission.id FROM roles role CROSS JOIN permissions permission WHERE role.enterprise_id=$1 AND permission.id IN ('host.read','telemetry.collector.read','telemetry.collector.manage') ON CONFLICT DO NOTHING", f.a.EnterpriseID)
	target := ResourceRef{Kind: "host", ID: collector.ResourceID}
	items, err := f.s.Builtins(ctx, f.a, target)
	if err != nil || len(items) != 5 {
		t.Fatalf("own builtin directory: %+v %v", items, err)
	}
	for _, item := range items {
		if (item.Definition.Key == "host_systemd_metrics" || item.Definition.Key == "host_windows_service_metrics") && item.Available {
			t.Fatal("missing service receiver or wrong platform was advertised as available")
		}
	}
	if _, err = f.s.Builtins(ctx, f.other, target); !errors.Is(err, ErrDenied) {
		t.Fatal("fixed collection list bypassed authoritative resource grant")
	}
	apply := func(input BuiltinInput) resource.ActionCommitResult {
		t.Helper()
		p, err := f.s.PreviewBuiltin(ctx, f.a, input, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		x := ActionExtension{Service: f.s}
		r := resource.Service{Store: f.s.Store, Actions: f.s.Actions, Extension: x}
		workflow := actionservice.Service{Store: f.s.Store, Idempotency: f.s.Actions.Idempotency, Resources: r}
		confirmation, err := workflow.Confirm(ctx, f.a.SubjectID.String(), uuid.NewString(), f.a.EnterpriseID, f.a.AuthorizationVersion, false, p.ActionRef, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		var result resource.ActionCommitResult
		if err = f.s.Store.InTx(ctx, func(q *db.Queries) error {
			var err error
			result, err = f.s.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, x.RevalidateAction, x.CommitAction)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := apply(BuiltinInput{Target: target, Key: "host_metrics", Enabled: true})
	if first.ResourceID != builtincollection.StableID(target.Kind, target.ID, "host_metrics") {
		t.Fatal("fixed instance identity changed")
	}
	self := apply(BuiltinInput{Target: target, Key: "collector_metrics", Enabled: true})
	row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: first.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	apply(BuiltinInput{Target: target, Key: "host_metrics", Enabled: false, ExpectedVersion: row.Version})
	hostRow, _ := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: first.ResourceID})
	selfRow, _ := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: self.ResourceID})
	if hostRow.DesiredEnabled || !selfRow.DesiredEnabled || hostRow.CollectorID.UUID != collectorID {
		t.Fatal("one own collection toggled another instance")
	}
	if _, err = f.s.PreviewBuiltin(ctx, f.a, BuiltinInput{Target: target, Key: "host_metrics", Enabled: true, ExpectedVersion: row.Version}, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatal("stale own collection version accepted")
	}
	f.exec("UPDATE hosts SET name=$2 WHERE id=$1", target.ID, target.ID.String())
	foreign := f.collector()
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Integration: "host_metrics", Target: target, CollectorID: foreign, Config: json.RawMessage(`{}`), Enabled: true}}); err == nil {
		t.Fatal("fixed own metrics accepted a foreign executor")
	}
	var targets int
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_targets WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&targets); err != nil || targets != 0 {
		t.Fatal("own collection created a parallel monitoring target")
	}
}
