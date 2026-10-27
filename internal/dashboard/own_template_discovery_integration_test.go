package dashboard

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresOwnTemplatesDiscoverNewBuiltinsWithoutLeakingPrivateResources(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	visible, private, distribution := uuid.New(), uuid.New(), uuid.New()
	execute("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,'Own template regression',$2,'fixture','v1','supported','[]',1)", distribution, distribution.String())
	for _, host := range []uuid.UUID{visible, private} {
		collector := uuid.New()
		execute("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,$3,'127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", host, f.actor.EnterpriseID, host.String())
		execute("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.actor.EnterpriseID, host, distribution, uuid.New())
		keys := []string{"host_process_metrics", "host_systemd_metrics"}
		if host == private {
			keys = []string{"host_metrics"}
		}
		for _, key := range keys {
			execute("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,$3,$3,'host',$4,$5,$6)", uuid.New(), f.actor.EnterpriseID, key, host, collector, f.actor.SubjectID)
		}
	}
	grant := uuid.New()
	execute("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", grant, f.actor.EnterpriseID, f.actor.SubjectID, visible)
	service := Service{Store: f.store, ProvideDefaults: true}
	if err := service.SyncTemplates(ctx, BuiltinTemplates()); err != nil {
		t.Fatal(err)
	}
	rows, err := service.List(ctx, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, row := range rows {
		names[row.Name] = true
	}
	if !names["Host processes"] || !names["Linux system services"] {
		t.Fatalf("new builtin defaults missing: %v", names)
	}
	if names["Host"] {
		t.Fatal("private Host collection leaked its managed template")
	}
	execute("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	rows, err = service.List(ctx, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Name == "Host processes" || row.Name == "Linux system services" {
			t.Fatal("revoked Host grant retained default dashboard discovery")
		}
	}
}

func TestPostgresManagedBindingsUseDefinitionReadPolicyAndKeepResourceGrants(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	execute("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'host.read' FROM role_bindings WHERE enterprise_id=$1 AND subject_type='user' AND subject_id=$2 ON CONFLICT DO NOTHING", f.actor.EnterpriseID, f.actor.SubjectID)
	host, grant := uuid.New(), uuid.New()
	execute("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Bound host','127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", host, f.actor.EnterpriseID)
	execute("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", grant, f.actor.EnterpriseID, f.actor.SubjectID, host)
	service := Service{Store: f.store}
	definition := BuiltinTemplates()[0]
	if err := service.SyncTemplates(ctx, []TemplateDefinition{definition}); err != nil {
		t.Fatal(err)
	}
	managed, err := service.ApplyManagedTemplate(ctx, f.actor.EnterpriseID, definition.Key)
	if err != nil {
		t.Fatal(err)
	}
	private := uuid.New()
	execute("INSERT INTO dashboards(id,enterprise_id,name,created_by,updated_by) VALUES($1,$2,'Private ordinary dashboard',$3,$3)", private, f.actor.EnterpriseID, f.actor.SubjectID)
	for _, dashboard := range []uuid.UUID{managed.ID, private} {
		execute("INSERT INTO dashboard_bindings(id,enterprise_id,dashboard_id,resource_type,resource_id,created_by) VALUES($1,$2,$3,'host',$4,$5)", uuid.New(), f.actor.EnterpriseID, dashboard, host, f.actor.SubjectID)
	}
	linked, err := service.ResourceBindings(ctx, f.actor, "host", host)
	if err != nil || len(linked.Items) != 1 || linked.Items[0].Dashboard.ID != managed.ID {
		t.Fatalf("managed definition hidden or private dashboard leaked: %+v %v", linked, err)
	}
	execute("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	if _, err = service.ResourceBindings(ctx, f.actor, "host", host); !errors.Is(err, ErrDenied) {
		t.Fatalf("managed definition bypassed Host data grant: %v", err)
	}
}
