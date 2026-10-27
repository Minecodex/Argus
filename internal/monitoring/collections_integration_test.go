package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (f *fixture) collector() uuid.UUID {
	f.t.Helper()
	host, collector, distribution := uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,labels_hash,role,control_path) VALUES($1,$2,'Plan8 isolated target','127.0.0.1',22,'development','linux',$3,'managed_host','direct')", host, f.a.EnterpriseID, bytes.Repeat([]byte{1}, 32))
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, host)
	f.exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,components,artifact_manifest,catalog_revision) VALUES($1,$2,$3,$4,'argus.collector_config/v1','supported',ARRAY['mysql','redis','nginx'],'[]',1)", distribution, "controlled fixture "+distribution.String(), configbundle.DistributionVersion, monitoringcatalog.CollectorVersion)
	f.exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','leaf','converged',1)", collector, f.a.EnterpriseID, host, distribution)
	return collector
}

func TestPostgresDNSRejectsNoopAndUnverifiedCollectorVersions(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['dns_check'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	input := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "DNS", TargetName: "DNS", Integration: "dns", TargetIdentity: json.RawMessage(`{"endpoint":"dns.test:53","hostnames":["reachable.test"]}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "keep"}}
	for _, version := range []string{"0.158.0", "0.162.0"} {
		f.exec("UPDATE collector_distribution_versions SET collector_version=$2 WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector, version)
		if _, err := f.s.Preview(t.Context(), f.a, input, uuid.NewString()); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("unverified Collector version %s accepted: %v", version, err)
		}
	}
	f.exec("UPDATE collector_distribution_versions SET collector_version=$2 WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector, monitoringcatalog.CollectorVersion)
	if _, err := f.s.Preview(t.Context(), f.a, input, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresCollectionsIndependentLifecycleAndPrivateCredentials(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	var ids []uuid.UUID
	for _, endpoint := range []string{"mysql-a:3306", "mysql-b:3306"} {
		identity, _ := json.Marshal(map[string]string{"endpoint": endpoint})
		result, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: endpoint, TargetName: endpoint, Integration: "mysql", TargetIdentity: identity, CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}, json.RawMessage(`{"username":"monitor","password":"sensitive-plan8-test-password"}`))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, result.ResourceID)
	}
	first, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: ids[0]})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: ids[1]})
	if err != nil {
		t.Fatal(err)
	}
	if first.CredentialID == second.CredentialID || first.TargetID == second.TargetID || first.CollectorID != second.CollectorID {
		t.Fatal("collection isolation lost")
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: first.ID, ExpectedVersion: first.Version, Collection: &CollectionInput{Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.Store.Queries.ListCollectorMonitoringCollections(ctx, db.ListCollectorMonitoringCollectionsParams{EnterpriseID: f.a.EnterpriseID, CollectorID: nullID(collector)})
	if err != nil || len(rows) != 2 {
		t.Fatalf("list: %v %v", rows, err)
	}
	for _, row := range rows {
		if row.ID == first.ID && (row.DesiredEnabled || row.DesiredRevision != 2 || row.EffectiveRevision != 0 || row.ApplyStatus != "pending") {
			t.Fatal("disabled instance incorrectly acknowledged")
		}
		if row.ID == second.ID && (!row.DesiredEnabled || row.Version != 1 || row.DesiredRevision != 1) {
			t.Fatal("other instance changed")
		}
	}
	credentials, err := f.s.Credentials.ListCredentials(ctx, f.a.EnterpriseID)
	if err != nil || len(credentials) != 0 {
		t.Fatalf("private credentials leaked: %v", err)
	}
	var public []byte
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT jsonb_agg(jsonb_build_object('preview',p.preview,'diff',p.diff,'plan',x.immutable_plan)) FROM pending_actions p JOIN pending_action_plans x ON x.pending_action_id=p.id WHERE p.enterprise_id=$1", f.a.EnterpriseID).Scan(&public); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(public, []byte("sensitive-plan8-test-password")) || bytes.Contains(public, []byte(`"password"`)) {
		t.Fatal("credential in action projection")
	}
	var count int
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE topic='monitoring.collection.changed' AND aggregate_id IN ($1,$2)", ids[0].String(), ids[1].String()).Scan(&count); err != nil || count != 3 {
		t.Fatalf("transactional outbox: %d %v", count, err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "archive", ID: first.ID, ExpectedVersion: 2, Collection: &CollectionInput{}}); err != nil {
		t.Fatal(err)
	}
	credential, err := f.s.Store.Queries.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: f.a.EnterpriseID, ID: first.CredentialID.UUID})
	if err != nil || credential.Status != "disabled" {
		t.Fatalf("archive credential: %+v %v", credential, err)
	}
}

func TestPostgresCollectionDraftRollbackAndReplay(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	input := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "mysql", TargetName: "mysql", Integration: "mysql", TargetIdentity: json.RawMessage(`{"endpoint":"db:3306"}`), CollectorID: collector, Config: json.RawMessage(`{}`), Enabled: true, CredentialUpdate: "replace"}}
	credentials := json.RawMessage(`{"username":"monitor","password":"test-only"}`)
	if _, err := f.s.PreviewWithCredentials(ctx, f.other, input, credentials, uuid.NewString()); !errors.Is(err, ErrDenied) {
		t.Fatalf("ungranted collector: %v", err)
	}
	var count int
	if err := f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM secrets WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed preview left secret: %d %v", count, err)
	}
	key := uuid.NewString()
	one, err := f.s.PreviewWithCredentials(ctx, f.a, input, credentials, key)
	if err != nil {
		t.Fatal(err)
	}
	two, err := f.s.PreviewWithCredentials(ctx, f.a, input, credentials, key)
	if err != nil || one.ID != two.ID {
		t.Fatalf("preview replay: %v", err)
	}
	if err = f.s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_credential_drafts WHERE enterprise_id=$1", f.a.EnterpriseID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replay created drafts: %d %v", count, err)
	}
	input.Collection.Integration = "clickhouse"
	if _, err = f.s.PreviewWithCredentials(ctx, f.a, input, nil, uuid.NewString()); err == nil {
		t.Fatal("deferred integration accepted")
	}
}
