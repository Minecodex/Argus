package toolgateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func verifySourceVersionPagesAreNotIndependentSources(t *testing.T, ctx context.Context, store *postgres.Store, actor authorization.Actor, application, endpoint uuid.UUID, reg *mcp.Registry) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	host, collector, distribution, generation, source := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	previous, err := store.Queries.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: actor.EnterpriseID, ID: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		exec("UPDATE monitoring_ingest_endpoints SET collector_id=$2,enabled=$3 WHERE id=$1", endpoint, previous.CollectorID, previous.Enabled)
		exec("DELETE FROM telemetry_sources WHERE id=$1 AND collector_id=$2", source, collector)
		exec("DELETE FROM collector_config_revisions WHERE collector_id=$1", collector)
		exec("DELETE FROM collector_instances WHERE id=$1", collector)
		exec("DELETE FROM collector_distribution_versions WHERE id=$1", distribution)
		exec("DELETE FROM hosts WHERE id=$1", host)
	}()
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Private SDK transport','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, actor.EnterpriseID)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,$2,'controlled','v1','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,26,26)", collector, actor.EnterpriseID, host, distribution, generation)
	exec("UPDATE monitoring_ingest_endpoints SET collector_id=$2,enabled=false WHERE id=$1", endpoint, collector)
	for revision := 1; revision <= 26; revision++ {
		exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,$3,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector, revision)
		exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,ingest_endpoint_id,ingest_endpoint_version) VALUES($1,$2,$3,$4,'application',$5,'otlp','otlp',ARRAY['metrics','logs','traces'],$6,decode(repeat('00',32),'hex'),'v1',$7,1)", source, actor.EnterpriseID, collector, generation, application, revision, endpoint)
	}
	call := mcp.Call{Enterprise: actor.EnterpriseID.String(), Subject: actor.SubjectID.String(), SubjectType: actor.SubjectType, ToolID: "observe.capabilities.get", Input: map[string]any{
		"resources": []any{map[string]any{"type": "application", "id": application.String()}}, "sections": []string{"sources"},
	}}
	seen := map[string]bool{}
	for page, expected := range []int{25, 1} {
		result, err := reg.Call(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		items := result.Structured["items"].([]any)
		section := items[0].(map[string]any)["sections"].(map[string]any)["sources"].(map[string]any)
		versions := section["items"].([]any)
		if len(versions) != expected || section["page_entry_count"] != float64(expected) || section["page_source_count"] != float64(1) || section["entry_kind"] != "source_version_selectors" || section["scope_note"] == "" {
			t.Fatal("version rows were counted as independent sources or total instead of page", page, section)
		}
		for _, raw := range versions {
			row := raw.(map[string]any)
			key := row["source_key"].(string)
			if row["source_id"] != source.String() || row["entry_kind"] != "source_version_selector" || row["installation_scope"] != "collector_generation_only" || row["current_installation"] != true || seen[key] {
				t.Fatal("stable source identity/version/generation meaning lost", row)
			}
			seen[key] = true
		}
		if page == 0 {
			if section["complete"] != false || section["next_cursor"] == "" {
				t.Fatal("first page lost its bounded continuation")
			}
			call.Input["cursor"] = section["next_cursor"]
		} else if section["complete"] != true {
			t.Fatal("final source page not complete")
		}
	}
	if len(seen) != 26 || !seen[fmt.Sprintf("%s:1", source)] || !seen[fmt.Sprintf("%s:26", source)] {
		t.Fatal("historical source revision selectors were collapsed or lost")
	}
	t.Log("One stable source retains 26 version selectors across25/1 pages; page sourcecount1 each, currentinstallationtrue despiteSDKendpointdisabled")
}
