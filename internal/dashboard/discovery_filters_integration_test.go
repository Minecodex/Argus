package dashboard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resourceref"
)

func TestPostgresDashboardDiscoveryFiltersPartialAndNoMatch(t *testing.T) {
	f := newAnalysisFixture(t)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	configured, empty, private := uuid.New(), uuid.New(), uuid.New()
	distribution, collector, generation, source := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'host.read' FROM role_bindings WHERE enterprise_id=$1 AND subject_id=$2 ON CONFLICT DO NOTHING", f.actor.EnterpriseID, f.actor.SubjectID)
	for _, id := range []uuid.UUID{configured, empty, private} {
		exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,$3,'127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", id, f.actor.EnterpriseID, id.String())
		if id != private {
			exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, id)
		}
	}
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,$2,'controlled','v1','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.actor.EnterpriseID, configured, distribution, generation)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,'Host collection','host_metrics','host',$3,$4,$5)", uuid.New(), f.actor.EnterpriseID, configured, collector, f.actor.SubjectID)
	exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'hostmetrics','hostmetrics',ARRAY['metrics'],1,decode(repeat('00',32),'hex'),'v1')", source, f.actor.EnterpriseID, collector, generation, configured)
	service := Service{Store: f.store, ProvideDefaults: true}
	if err := service.SyncTemplates(t.Context(), BuiltinTemplates()); err != nil {
		t.Fatal(err)
	}
	base := DiscoveryInput{ResourceTypes: []string{"host"}, Signals: []string{"metrics"}, Integrations: []string{"host_metrics"}, ManagedOnly: true, Limit: 50}
	for _, c := range []struct {
		name                         string
		refs                         []resourceref.Ref
		signals, integrations, types []string
		want                         string
		empty                        bool
	}{
		{"applicable", []resourceref.Ref{{Type: "host", ID: configured}}, nil, nil, nil, "applicable", false},
		{"partial", []resourceref.Ref{{Type: "host", ID: configured}, {Type: "host", ID: empty}}, nil, nil, nil, "partial", false},
		{"unsupported resource", []resourceref.Ref{{Type: "host", ID: empty}}, nil, nil, nil, "", true},
		{"signal no match", nil, []string{"logs"}, nil, nil, "", true},
		{"integration no match", nil, nil, []string{"mysql"}, nil, "", true},
		{"type no match", nil, nil, nil, []string{"application"}, "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			input := base
			input.Resources = c.refs
			if c.signals != nil {
				input.Signals = c.signals
			}
			if c.integrations != nil {
				input.Integrations = c.integrations
			}
			if c.types != nil {
				input.ResourceTypes = c.types
			}
			page, err := service.Discover(t.Context(), f.actor, input)
			if err != nil {
				t.Fatal(err)
			}
			if c.empty {
				if len(page.Items) != 0 || !page.Complete || page.HasMore {
					t.Fatal("no match was not a complete empty directory", page)
				}
				return
			}
			if len(page.Items) == 0 {
				t.Fatal("configured authorized host lost its managed dashboard")
			}
			for _, item := range page.Items {
				if item.Suitability != c.want || item.SamplingPerformed || item.DataState != "not_checked" {
					t.Fatal("applicability mixed with sample health", item)
				}
			}
		})
	}
	if _, err := service.Discover(t.Context(), f.actor, DiscoveryInput{Resources: []resourceref.Ref{{Type: "host", ID: private}}}); err == nil {
		t.Fatal("private resource entered applicability selection")
	}
}
