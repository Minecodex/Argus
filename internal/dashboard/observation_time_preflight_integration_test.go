package dashboard

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Actual PostgreSQL grants/Run ledger and the production native entrypoint;
// only the final data backend is controlled. Invalid time must reserve nothing.
func TestPostgresNativeInvalidTimePreservesRunBudgetForCorrection(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "检查本轮查询预算")
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	host, distribution, collector, generation, source := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'host.read' FROM role_bindings WHERE enterprise_id=$1 AND subject_id=$2", f.actor.EnterpriseID, f.actor.SubjectID)
	exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'conversation.use' FROM role_bindings WHERE enterprise_id=$1 AND subject_id=$2 ON CONFLICT DO NOTHING", f.actor.EnterpriseID, f.actor.SubjectID)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Window preflight','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, f.actor.EnterpriseID)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, host)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,$2,'controlled','v1','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.actor.EnterpriseID, host, distribution, generation)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'hostmetrics','hostmetrics',ARRAY['metrics'],1,decode(repeat('00',32),'hex'),'v1')", source, f.actor.EnterpriseID, collector, generation, host)
	setTestBudget(t, f, budgetCost{1000, 8192, 3, 100}, 20)
	before, err := f.service.RunBudget(t.Context(), f.actor)
	if err != nil {
		t.Fatal(err)
	}
	backend := &countedBudgetBackend{}
	registry := mcp.NewRegistry()
	if err := (telemetry.Tools{Service: telemetry.Service{Store: f.store, Engine: backend, ModelQueryMeter: func(a telemetry.Actor, base telemetry.EngineQueryBackend) telemetry.EngineQueryBackend {
		return MeterObservationBackend(f.store, a, base)
	}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC().Truncate(time.Second)
	call := mcp.Call{ToolID: "telemetry.promql.query", Enterprise: f.actor.EnterpriseID.String(), Subject: f.actor.SubjectID.String(), SubjectType: "user", Caller: "model", RunID: f.actor.RunID.String(), Input: map[string]any{
		"query": "up", "from": end.Format(time.RFC3339), "to": end.Format(time.RFC3339), "max_scan_bytes": 1000, "max_result_bytes": 4096, "limit": 100,
		"scope": map[string]any{"mode": "explicit", "resources": []any{map[string]any{"type": "host", "id": host.String()}}, "source_keys": []string{source.String() + ":1"}},
	}}
	_, err = registry.Call(t.Context(), call)
	var safe toolruntime.Error
	if !errors.As(err, &safe) || safe.Kind != "TOOL_QUERY_INVALID" || safe.Details["budget_reserved"] != false || backend.calls.Load() != 0 {
		t.Fatal("invalid native time reached actual Run meter/data backend", err, backend.calls.Load())
	}
	after, err := f.service.RunBudget(t.Context(), f.actor)
	if err != nil || before != after {
		t.Fatal("invalid time changed actual persistent query budget", before, after, err)
	}
	call.Input["from"] = end.Add(-time.Minute).Format(time.RFC3339)
	if _, err := registry.Call(t.Context(), call); err != nil || backend.calls.Load() != 1 {
		t.Fatal("corrected same-scope/window request could not use the preserved budget", err, backend.calls.Load())
	}
	after, err = f.service.RunBudget(t.Context(), f.actor)
	if err != nil || after.CallsRemaining != before.CallsRemaining-1 {
		t.Fatal("corrected native request was not settled exactly once", before, after, err)
	}
	t.Log("Actual native invalid equal window reserves no PostgreSQL budget; corrected original positive window executes and settles once")
}
