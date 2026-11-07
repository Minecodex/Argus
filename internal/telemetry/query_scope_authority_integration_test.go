package telemetry

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestPostgresQueryScopeRejectsInvalidAndForeignSelectionsWithoutWidening(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "query_scope", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, user, role := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	allowed, hidden, distribution := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,timezone) VALUES($1,'Query scope','UTC')", enterprise)
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Readers')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Scope reader')", user, enterprise, department, user.String())
	exec("INSERT INTO permissions(id,description,registry_version) VALUES('host.read','Read Host',$1) ON CONFLICT DO NOTHING", authorization.PermissionRegistryVersion)
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Host reader')", role, enterprise)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'host.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'scope','scope','v1','supported','[]',1)", distribution, distribution.String())
	keys := map[uuid.UUID]string{}
	for _, host := range []uuid.UUID{allowed, hidden} {
		collector, generation, source := uuid.New(), uuid.New(), uuid.New()
		exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,$3,'127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, enterprise, host.String())
		exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, enterprise, host, distribution, generation)
		exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
		exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'host_metrics','hostmetrics',ARRAY['metrics','logs','traces'],1,decode(repeat('00',32),'hex'),'v1')", source, enterprise, collector, generation, host)
		keys[host] = source.String() + ":1"
	}
	grant := uuid.New()
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", grant, enterprise, user, allowed)
	actor := Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	service := Service{Store: store}
	valid := QueryScope{Mode: observationscope.Explicit, Resources: []resourceref.Ref{{Type: "host", ID: allowed}}}
	for _, signal := range []string{"metrics", "logs", "traces"} {
		snapshot, sourceKeys, err := service.ResolveStructuredQueryScope(ctx, actor, valid, signal)
		if err != nil || len(snapshot.Resources) != 1 || len(sourceKeys) != 1 || sourceKeys[0] != keys[allowed] {
			t.Fatalf("positive scoped source fixture invalid (%s): %+v %v %v", signal, snapshot, sourceKeys, err)
		}
		for name, scope := range map[string]QueryScope{
			"unknown type":         {Mode: observationscope.Explicit, Resources: []resourceref.Ref{{Type: "dashboard", ID: allowed}}},
			"zero id":              {Mode: observationscope.Explicit, Resources: []resourceref.Ref{{Type: "host", ID: uuid.Nil}}},
			"hidden id":            {Mode: observationscope.Explicit, Resources: []resourceref.Ref{{Type: "host", ID: hidden}}},
			"missing id":           {Mode: observationscope.Explicit, Resources: []resourceref.Ref{{Type: "host", ID: uuid.New()}}},
			"wrong object type":    {Mode: observationscope.Explicit, Resources: []resourceref.Ref{{Type: "application", ID: allowed}}},
			"source outside grant": {Mode: observationscope.Explicit, Resources: valid.Resources, SourceKeys: []string{keys[hidden]}},
			"unknown source":       {Mode: observationscope.Explicit, Resources: valid.Resources, SourceKeys: []string{uuid.NewString() + ":1"}},
			"malformed source":     {Mode: observationscope.Explicit, Resources: valid.Resources, SourceKeys: []string{"not-a-source"}},
			"all with explicit id": {Mode: observationscope.AllAuthorized, Resources: valid.Resources},
		} {
			_, selected, err := service.ResolveStructuredQueryScope(ctx, actor, scope, signal)
			if err == nil || len(selected) != 0 {
				t.Fatalf("%s/%s widened selection: %v %v", signal, name, selected, err)
			}
		}
		for _, legacy := range [][]uuid.UUID{{allowed}, {}} {
			if _, _, err := service.ResolveQueryInput(ctx, actor, legacy, &valid, signal); !errors.Is(err, ErrQueryInvalid) {
				t.Fatal("scope and resource_ids were combined", signal, err)
			}
		}
		empty := QueryScope{Mode: observationscope.Explicit, Resources: []resourceref.Ref{}}
		if snapshot, selected, err := service.ResolveQueryInput(ctx, actor, nil, &empty, signal); err != nil || len(snapshot.DataIDs()) != 0 || len(selected) != 0 {
			t.Fatal("explicit empty became all authorized", signal, selected, err)
		}
		noSources := valid
		noSources.SourceKeys = []string{}
		if snapshot, selected, err := service.ResolveStructuredQueryScope(ctx, actor, noSources, signal); err != nil || len(snapshot.DataIDs()) != 0 || len(selected) != 0 {
			t.Fatal("explicit empty source list became all", signal, selected, err)
		}
	}
	exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	if _, _, err := service.ResolveStructuredQueryScope(ctx, actor, valid, "metrics"); !errors.Is(err, ErrDenied) {
		t.Fatal("revoked resource remained usable", err)
	}
}
