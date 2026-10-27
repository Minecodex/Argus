package dashboard

import (
	"bytes"
	"errors"
	"github.com/google/uuid"
	"testing"
)

func TestPostgresExecutionScopeModesAndApplicationDependencies(t *testing.T) {
	f := newAnalysisFixture(t)
	app, target := uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Application','production',$4)", app, f.actor.EnterpriseID, app.String(), f.actor.SubjectID)
	exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'Database','database','mysql','{\"endpoint\":\"db:3306\"}',$3)", target, f.actor.EnterpriseID, f.actor.SubjectID)
	for kind, id := range map[string]uuid.UUID{"application": app, "monitoring_target": target} {
		exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,$4,$5)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, kind, id)
	}
	exec("INSERT INTO monitoring_application_bindings(id,enterprise_id,application_id,target_type,target_id) VALUES($1,$2,$3,'monitoring_target',$4)", uuid.New(), f.actor.EnterpriseID, app, target)
	for _, tc := range []struct {
		name  string
		input ExecutionInput
		count int
	}{
		{"all", ExecutionInput{ResourceMode: "all_authorized"}, 2},
		{"explicit empty", ExecutionInput{ResourceMode: "explicit", ResourceIDs: []uuid.UUID{}}, 0},
		{"application dependencies", ExecutionInput{ApplicationMode: "explicit", ApplicationIDs: []uuid.UUID{app}}, 2},
		{"empty applications", ExecutionInput{ApplicationMode: "explicit", ApplicationIDs: []uuid.UUID{}}, 0},
		{"typed application dependencies", ExecutionInput{ApplicationMode: "explicit", ApplicationIDs: []uuid.UUID{app}, ResourceTypes: []string{"monitoring_target"}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := resolveExecutionScope(t.Context(), f.store.Queries, f.actor, tc.input)
			if err != nil || len(snapshot.Resources) != tc.count || len(snapshot.Signature) != 64 {
				t.Fatalf("scope %+v: %v", snapshot, err)
			}
		})
	}
	if _, err := resolveExecutionScope(t.Context(), f.store.Queries, f.actor, ExecutionInput{ResourceMode: "all_authorized", ResourceIDs: []uuid.UUID{target}}); err == nil {
		t.Fatal("ambiguous all plus IDs accepted")
	}
}

func TestPostgresTypedSourcesDoNotUseAnotherResourceKindWithTheSameUUID(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	id, collector, distribution, generation, source := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,labels_hash,role,control_path) VALUES($1,$2,'Private host','127.0.0.1',22,'development','linux',$3,'managed_host','direct')", id, f.actor.EnterpriseID, bytes.Repeat([]byte{1}, 32))
	exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Granted application','production',$4)", id, f.actor.EnterpriseID, id.String(), f.actor.SubjectID)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, id)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,'Typed-scope fixture',$2,'fixture','v1','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,effective_revision,desired_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.actor.EnterpriseID, id, distribution, generation)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,ARRAY[$3]::uuid[],'{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector, uuid.New())
	exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'hostmetrics','hostmetrics',ARRAY['metrics'],1,decode(repeat('00',32),'hex'),'v1')", source, f.actor.EnterpriseID, collector, generation, id)
	snapshot, err := resolveExecutionScope(ctx, f.store.Queries, f.actor, ExecutionInput{ResourceMode: "explicit", ResourceIDs: []uuid.UUID{id}, ResourceTypes: []string{"application"}})
	if err != nil || len(snapshot.Resources) != 1 || snapshot.Resources[0].Kind != "application" {
		t.Fatalf("typed scope: %+v %v", snapshot, err)
	}
	rows, err := resolveSources(ctx, f.store.Queries, f.actor, SourceBinding{SourceType: "hostmetrics", CapabilityVersion: "v1"}, "metrics", snapshot.Refs())
	if err != nil || len(rows) != 0 {
		t.Fatalf("application grant selected host telemetry: %+v %v", rows, err)
	}
	// Positive SQL control: the host source exists, but is outside the granted application scope.
	rows, err = resolveSources(ctx, f.store.Queries, f.actor, SourceBinding{SourceType: "hostmetrics", CapabilityVersion: "v1"}, "metrics", []ResourceScope{{Type: "host", ID: id}})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("ungranted typed host source accepted: %+v %v", rows, err)
	}
	var count int
	if err = f.store.Pool.QueryRow(ctx, "SELECT count(*) FROM telemetry_sources WHERE enterprise_id=$1 AND id=$2 AND resource_type='host'", f.actor.EnterpriseID, source).Scan(&count); err != nil || count != 1 {
		t.Fatalf("fixture source unavailable: %d %v", count, err)
	}
}
