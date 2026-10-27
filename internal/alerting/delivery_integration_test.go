package alerting

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestPostgresCollectorDeliveryReportOutboxAlertIsolationAndReplay(t *testing.T) {
	migrations := os.Getenv("ARGUS_PLAN8_DELIVERY_MIGRATIONS")
	if migrations == "" {
		migrations = filepath.Join("..", "..", "migrations", "postgresql")
	}
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "delivery_alert", migrations)
	ctx := t.Context()
	enterprise, user, role, host, collector, distribution := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Delivery evidence',$2,'UTC')", enterprise, enterprise.String())
	department := uuid.New()
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Operations')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Operator')", user, enterprise, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Delivery operator')", role, enterprise)
	for _, permission := range []string{"host.read", "alert.read", "alert.policy.manage"} {
		exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", permission, authorization.PermissionRegistry[permission], authorization.PermissionRegistryVersion)
		exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,$2)", role, permission)
	}
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Collector owner','192.0.2.1',22,'linux','managed_host','direct','production',decode(repeat('00',32),'hex'))", host, enterprise)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), enterprise, user, host)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,components,artifact_manifest,catalog_revision) VALUES($1,$2,'test','0.159.0','argus.otelcol_bundle/v3','supported','{}','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged')", collector, enterprise, host, distribution)
	identity := telemetry.TrustedIdentity{EnterpriseID: enterprise, CollectorID: collector, ResourceID: host, ResourceType: "host"}
	delivery := telemetry.CollectorDeliveryService{Store: store}
	actor := Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	service := Service{Store: store}
	at := time.Now().UTC().Add(-time.Minute)
	failureAt := at.Add(time.Second)
	report := telemetry.CollectorDeliveryReport{SchemaVersion: "argus.collector_delivery/v1", InstanceID: uuid.NewString(), Sequence: 1, ObservedSince: at, ObservedAt: at.Add(2 * time.Second), LastFailureAt: &failureAt, TerminalFailures: telemetry.DeliveryCounts{LogRecords: 3}, ReceiverRefusals: telemetry.DeliveryCounts{LogRecords: 12}, RetryLimitSeconds: 300, Attribution: "collector_shared_pipeline_exact_targets_unknown"}
	if err := delivery.ObserveDeliveryReport(ctx, identity, report); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueCollectorDeliveryAlerts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := delivery.ObserveDeliveryReport(ctx, identity, report); err != nil {
		t.Fatal("same report should be idempotent", err)
	}
	report.Sequence = 2
	report.ObservedAt = at.Add(3 * time.Second)
	if err := delivery.ObserveDeliveryReport(ctx, identity, report); err != nil {
		t.Fatal(err)
	}
	report.Sequence = 3
	report.TerminalFailures.LogRecords = 4
	report.ObservedAt = at.Add(4 * time.Second)
	if err := delivery.ObserveDeliveryReport(ctx, identity, report); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueCollectorDeliveryAlerts(ctx); err != nil {
		t.Fatal(err)
	}
	var tasks []uuid.UUID
	rows, err := store.Pool.Query(ctx, "SELECT id FROM runtime_tasks WHERE enterprise_id=$1 AND queue=$2 ORDER BY created_at,id", enterprise, telemetry.CollectorDeliveryQueue)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, id)
	}
	rows.Close()
	if len(tasks) != 1 {
		t.Fatal("heartbeats or duplicate reports created extra alert jobs", len(tasks))
	}
	worker := DeliveryGapWorker{Service: service}
	for _, id := range tasks {
		exec("UPDATE runtime_tasks SET status='running',lease_owner='delivery-test',lease_until=now()+interval '1 hour',fence_token=1 WHERE id=$1", id)
		task := runtime.Task{RuntimeTask: db.RuntimeTask{ID: id, EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, LeaseOwner: pgtype.Text{String: "delivery-test", Valid: true}, FenceToken: 1, Payload: mustDeliveryJSON(t, map[string]any{"report_id": uuid.NewSHA1(collector, []byte("delivery:"+uuid.MustParse(report.InstanceID).String()))})}}
		if err = worker.Handle(ctx, task); err != nil {
			t.Fatal(err)
		}
		if err = worker.Handle(ctx, task); err != nil {
			t.Fatal("replayed task should not duplicate alerts", err)
		}
		exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", id)
	}
	page, err := service.Instances(ctx, actor, ListFilter{}, false)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("actual shared alert read did not expose one gap", err, len(page.Items))
	}
	instance := page.Items[0]
	if instance.State.DeliveryGap == nil || instance.State.DeliveryGap.TerminalFailures.LogRecords != 4 || instance.State.DeliveryGap.ReceiverRefusals.LogRecords != 12 || instance.Lifecycle != "firing" || len(instance.ResourceIDs) != 1 || instance.ResourceIDs[0] != host {
		t.Fatal("gap count, attempts or scope changed", instance)
	}
	events, err := store.Queries.ListAlertEvents(ctx, db.ListAlertEventsParams{EnterpriseID: enterprise, InstanceID: instance.ID})
	if err != nil || len(events) != 1 {
		t.Fatal("task replay duplicated lifecycle event", err, len(events))
	}
	// A successful new metric read cannot erase already lost records.
	report.Sequence = 4
	report.ObservedAt = at.Add(5 * time.Second)
	if err = delivery.ObserveDeliveryReport(ctx, identity, report); err != nil {
		t.Fatal(err)
	}
	page, err = service.Instances(ctx, actor, ListFilter{}, false)
	if err != nil || len(page.Items) != 1 || page.Items[0].Lifecycle != "firing" {
		t.Fatal("fresh heartbeat falsely recovered history", err)
	}
	bad := report
	bad.Sequence = 5
	bad.TerminalFailures.LogRecords = 0
	if err = delivery.ObserveDeliveryReport(ctx, identity, bad); !errors.Is(err, telemetry.ErrDeliveryReport) {
		t.Fatal("decreasing evidence accepted", err)
	}
	// A failed receipt update cannot acknowledge a counter or its Outbox bit.
	exec("CREATE FUNCTION fail_delivery_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'owned delivery receipt failure';END $$")
	exec("CREATE TRIGGER fail_delivery_receipt BEFORE UPDATE ON collector_delivery_reports FOR EACH ROW EXECUTE FUNCTION fail_delivery_receipt()")
	bad = report
	bad.Sequence = 5
	bad.TerminalFailures.LogRecords = 5
	if err = delivery.ObserveDeliveryReport(ctx, identity, bad); err == nil {
		t.Fatal("failed receipt was acknowledged")
	}
	exec("DROP TRIGGER fail_delivery_receipt ON collector_delivery_reports")
	ledger, err := store.Queries.ListCollectorDeliveryReports(ctx, db.ListCollectorDeliveryReportsParams{EnterpriseID: enterprise, CollectorID: collector})
	if err != nil || len(ledger) != 1 || ledger[0].Sequence != 4 {
		t.Fatal("failed transaction advanced receipt", err)
	}
	if err = delivery.ObserveDeliveryReport(ctx, identity, bad); err != nil {
		t.Fatal(err)
	}
	exec("CREATE FUNCTION fail_delivery_task() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.queue='alert.collector_delivery' THEN RAISE EXCEPTION 'owned delivery outbox failure';END IF;RETURN NEW;END $$")
	exec("CREATE TRIGGER fail_delivery_task BEFORE INSERT ON runtime_tasks FOR EACH ROW EXECUTE FUNCTION fail_delivery_task()")
	if err = service.EnqueueCollectorDeliveryAlerts(ctx); err == nil {
		t.Fatal("failed Outbox dispatch was acknowledged")
	}
	ledger, err = store.Queries.ListCollectorDeliveryReports(ctx, db.ListCollectorDeliveryReportsParams{EnterpriseID: enterprise, CollectorID: collector})
	if err != nil || len(ledger) != 1 || ledger[0].Sequence != 5 || !ledger[0].PendingAlert {
		t.Fatal("failed dispatch erased durable Outbox", err)
	}
	exec("DROP TRIGGER fail_delivery_task ON runtime_tasks")
	if err = service.EnqueueCollectorDeliveryAlerts(ctx); err != nil {
		t.Fatal("Outbox did not recover", err)
	}
	exec("UPDATE runtime_tasks SET status='failed',updated_at=now()-interval '6 minutes' WHERE enterprise_id=$1 AND queue=$2 AND status='pending'", enterprise, telemetry.CollectorDeliveryQueue)
	if err = service.EnqueueCollectorDeliveryAlerts(ctx); err != nil {
		t.Fatal("exhausted alert intent did not become retryable", err)
	}
	var pending int64
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM runtime_tasks WHERE enterprise_id=$1 AND queue=$2 AND status='pending'", enterprise, telemetry.CollectorDeliveryQueue).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("exhausted intent was lost or concurrently duplicated", pending, err)
	}

	if _, err = service.Preview(ctx, actor, Mutation{Kind: "policy", Operation: "delete", ID: instance.PolicyID, ExpectedVersion: 1}, uuid.NewString()); !errors.Is(err, ErrDenied) {
		t.Fatal("managed gap policy was editable", err)
	}
	exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_id=$2", enterprise, user)
	page, err = service.Instances(ctx, actor, ListFilter{}, false)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("revoked resource exposed delivery evidence", err)
	}
	if _, _, err = service.Instance(ctx, actor, instance.ID); !errors.Is(err, ErrDenied) {
		t.Fatal("direct alert detail bypassed revoked scope", err)
	}
	other := identity
	other.EnterpriseID = uuid.New()
	if err = delivery.ObserveDeliveryReport(ctx, other, bad); err == nil {
		t.Fatal("cross-enterprise receipt accepted")
	}
}
func mustDeliveryJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
