package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type quantitySemanticsBackend struct{ name, kind, unit, source string }

func (b *quantitySemanticsBackend) DiscoverData(_ context.Context, r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	if r.Kind != "metric_variants" && r.Kind != "metrics" {
		return telemetry.DataCatalogResult{}, fmt.Errorf("source-qualified metadata requires metrics catalog, got %s", r.Kind)
	}
	return telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{{Name: b.name, Type: b.kind, Unit: b.unit, SourceType: "hostmetrics", SourceKeys: r.SourceKeys, Labels: []string{}}}, Complete: true}, nil
}

func (b *quantitySemanticsBackend) ExecuteEngineQuery(_ context.Context, r queryengine.Request) (queryengine.Result, error) {
	key := r.Scope.SourceKeys[0]
	if b.source != "" {
		key = b.source
	}
	result := queryengine.Result{Language: r.Language, ResultType: "vector", Data: []map[string]any{{"metric": map[string]string{"__name__": b.name}, "value": []any{r.End.Unix(), "0.7"}}}, Meta: queryengine.QueryMeta{ReturnedRows: 1, InputMetricsOrigin: metricmeta.Origin, InputMetricsComplete: true, ResultIsOriginalMetric: true, InputMetrics: []metricmeta.Descriptor{{Name: b.name, Type: b.kind, Unit: b.unit, StorageType: b.kind, SourceKey: key}}}}
	if r.Step > 0 {
		result.ResultType = "matrix"
		result.Data = []map[string]any{{"metric": map[string]string{"__name__": b.name}, "values": []any{[]any{r.Start.Unix(), "0.7"}, []any{r.End.Unix(), "0.7"}}}}
	}
	return result, nil
}
func TestPostgresNativeMetricQuantityUsesAuthorizedSourceDefinition(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "读取内存指标语义")
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
	backend := &quantitySemanticsBackend{name: "system_memory_utilization", kind: "gauge", unit: "1"}
	registry := mcp.NewRegistry()
	if err := (telemetry.Tools{Service: telemetry.Service{Store: f.store, Engine: backend}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC().Truncate(time.Second)
	call := mcp.Call{ToolID: "telemetry.promql.query", Enterprise: f.actor.EnterpriseID.String(), Subject: f.actor.SubjectID.String(), SubjectType: "user", Caller: "model", RunID: f.actor.RunID.String(), Input: map[string]any{"query": backend.name, "from": end.Add(-time.Minute).Format(time.RFC3339), "to": end.Format(time.RFC3339), "scope": map[string]any{"mode": "explicit", "resources": []any{map[string]any{"type": "host", "id": host.String()}}, "source_keys": []string{source.String() + ":1"}}}}
	for _, change := range []string{"none", "unknown_name", "wrong_type", "wrong_unit", "foreign_metadata"} {
		backend.name, backend.kind, backend.unit, backend.source = "system_memory_utilization", "gauge", "1", ""
		switch change {
		case "unknown_name":
			backend.name = "status_code"
		case "wrong_type":
			backend.kind = "counter"
		case "wrong_unit":
			backend.unit = "By"
		case "foreign_metadata":
			backend.source = uuid.NewString() + ":1"
		}
		call.Input["query"] = backend.name
		result, err := registry.Call(t.Context(), call)
		if err != nil {
			t.Fatal(change, err)
		}
		meta := result.Structured["argus_meta"].(map[string]any)
		meanings, present := meta["input_metric_semantics"].([]telemetry.QueryInputMetricSemantics)
		if (present && len(meanings) == 1) != (change == "none") {
			t.Fatal("native quantity semantics escaped current exactsource", change, meanings)
		}
	}
	backend.name, backend.kind, backend.unit, backend.source = "system_memory_utilization", "gauge", "1", ""
	service := telemetry.Service{Store: f.store, Engine: backend}
	catalog, err := service.ObservationCatalog(t.Context(), telemetry.Actor{EnterpriseID: f.actor.EnterpriseID, SubjectID: f.actor.SubjectID, SubjectType: "user", AuthorizationVersion: f.actor.AuthorizationVersion}, telemetry.ObservationCatalogInput{Scope: telemetry.QueryScope{Mode: "explicit", Resources: []resourceref.Ref{{Type: "host", ID: host}}, SourceKeys: []string{source.String() + ":1"}}, Signal: "metrics", Kind: "metrics", From: end.Add(-time.Minute), To: end, Limit: 10}, nil)
	if err != nil || len(catalog.Metrics) != 1 || catalog.Metrics[0].Semantics == nil || catalog.Metrics[0].Semantics.QuantityKind != "ratio" {
		t.Fatal("catalog/native quantity meaning diverged", catalog, err)
	}
	authoringRuntime := Runtime{Store: f.store, Backend: backend}
	input := CatalogInput{SourceBinding: SourceBinding{SourceType: "hostmetrics", CapabilityVersion: "v1"}, Signal: "metrics", Kind: "metrics", ResourceMode: "explicit", ResourceTypes: []string{"host"}, ResourceIDs: []uuid.UUID{host}, From: end.Add(-time.Minute), To: end, Limit: 10}
	authoring, err := authoringRuntime.Catalog(t.Context(), f.actor, input)
	if err != nil || len(authoring.Metrics) != 1 || authoring.Metrics[0].Semantics == nil || len(authoring.Metrics[0].Recipes) == 0 || *authoring.Metrics[0].Semantics != *catalog.Metrics[0].Semantics {
		t.Fatal("authoring catalog did not share exact authorized quantity definition and published recipe", authoring, err)
	}
	spec := EmptySpec()
	for _, definition := range BuiltinTemplates() {
		if definition.Key != "host.overview" {
			continue
		}
		for _, panel := range definition.Spec.Panels {
			if panel.ID == "memory" {
				spec.Panels = []Panel{panel}
			}
		}
	}
	if len(spec.Panels) != 1 {
		t.Fatal("published Host memory recipe missing")
	}
	spec.Panels[0].Unit = "percent"
	parameters := ExecutionInput{ResourceMode: "explicit", ResourceTypes: []string{"host"}, ResourceIDs: []uuid.UUID{host}}
	report, _, sampleErr := authoringRuntime.verifySample(t.Context(), f.actor, spec, parameters)
	if !errors.Is(sampleErr, ErrInvalid) || report.Valid || !slices.ContainsFunc(report.Issues, func(issue Issue) bool { return issue.Code == "QUANTITY_DISPLAY_MISMATCH" }) {
		t.Fatal("actual PG sample verifier did not reject wrong raw-ratio display", report, sampleErr)
	}
	spec.Panels[0].Unit = "percent_ratio"
	report, _, sampleErr = authoringRuntime.verifySample(t.Context(), f.actor, spec, parameters)
	if sampleErr != nil || !report.Valid {
		t.Fatal("correct ratio formatter failed actual PG sample validation", report, sampleErr)
	}
	otherHost := uuid.New()
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Other authorized Host','127.0.0.2',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", otherHost, f.actor.EnterpriseID)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, otherHost)
	specJSON, _ := json.Marshal(spec)
	publication := Service{Store: f.store, Verifier: authoringRuntime, Actions: resource.PendingActionService{Store: f.store, Key: []byte(strings.Repeat("k", 32)), Idempotency: postgres.Idempotency{Key: []byte(strings.Repeat("k", 32))}}}
	draft, draftErr := publication.CreateDraft(t.Context(), f.actor, DraftInput{Name: "Bounded publication scope", Spec: specJSON})
	if draftErr != nil {
		t.Fatal(draftErr)
	}
	preview, previewErr := publication.PreviewPublish(t.Context(), f.actor, draft.ID, draft.DraftVersion, uuid.NewString(), parameters)
	if previewErr != nil {
		t.Fatal("scoped publication preview failed", previewErr)
	}
	var public struct {
		Sample string `json:"sample_json"`
	}
	var captured struct {
		Execution Execution `json:"execution"`
	}
	if json.Unmarshal(preview.Action.Preview, &public) != nil || json.Unmarshal([]byte(public.Sample), &captured) != nil || len(captured.Execution.Resources) != 1 || captured.Execution.Resources[0].ID != host {
		t.Fatal("publication verification widened beyond requested Host", string(preview.Action.Preview))
	}
	exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.actor.EnterpriseID)
	if _, err := registry.Call(t.Context(), call); err == nil {
		t.Fatal("revoked source retained native quantity/data access")
	}
	if _, err := authoringRuntime.Catalog(t.Context(), f.actor, input); err == nil {
		t.Fatal("revoked source retained authoring guidance/data access")
	}
	t.Log("Actual PostgreSQL current source/permission resolution with controlled metric backend: exact builtin ratio only, unknown/typeunit/foreign metadata and revocation stay separate")
}
