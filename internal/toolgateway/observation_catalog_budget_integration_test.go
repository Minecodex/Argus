package toolgateway

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type observationCatalogBudgetBackend struct {
	requests []telemetry.DataCatalogRequest
	read     func(telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error)
}

func (*observationCatalogBudgetBackend) ExecuteEngineQuery(context.Context, queryengine.Request) (queryengine.Result, error) {
	return queryengine.Result{}, errors.New("unexpected sample query in directory test")
}

func (b *observationCatalogBudgetBackend) DiscoverData(_ context.Context, in telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	b.requests = append(b.requests, in)
	return b.read(in)
}

// Authorization and source resolution use actual PostgreSQL. Only the metered
// data backend is controlled, so budget propagation can be checked precisely.
func verifyObservationCatalogBudgetAndUnobserved(t *testing.T, store *postgres.Store, actor authorization.Actor) {
	t.Helper()
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	host, collector, distribution, generation, source, collection := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	target, grant := uuid.New(), uuid.New()
	defer func() {
		exec("DELETE FROM telemetry_sources WHERE id=$1 AND collector_id=$2", source, collector)
		exec("DELETE FROM monitoring_collection_revisions WHERE collection_id=$1", collection)
		exec("DELETE FROM monitoring_collections WHERE id=$1", collection)
		exec("DELETE FROM collector_config_revisions WHERE collector_id=$1", collector)
		exec("DELETE FROM collector_instances WHERE id=$1", collector)
		exec("DELETE FROM collector_distribution_versions WHERE id=$1", distribution)
		exec("DELETE FROM hosts WHERE id=$1", host)
		exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
		exec("DELETE FROM monitoring_targets WHERE id=$1", target)
	}()
	exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'Directory budget target','database','mysql','{}',$3)", target, actor.EnterpriseID, actor.SubjectID)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", grant, actor.EnterpriseID, actor.SubjectID, target)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Private directory transport','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, actor.EnterpriseID)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,$2,'controlled','v1','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, actor.EnterpriseID, host, distribution, generation)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,effective_collector_id,created_by) VALUES($1,$2,'Directory semantics','mysql','monitoring_target',$3,$4,$4,$5)", collection, actor.EnterpriseID, target, collector, actor.SubjectID)
	exec("INSERT INTO monitoring_collection_revisions(enterprise_id,collection_id,revision,integration_version,config,desired_enabled,collector_id,assignment_epoch,created_by) VALUES($1,$2,1,'1','{}',true,$3,1,$4)", actor.EnterpriseID, collection, collector, actor.SubjectID)
	exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch) VALUES($1,$2,$3,$4,'monitoring_target',$5,'mysql','mysql',ARRAY['metrics'],1,decode(repeat('00',32),'hex'),'v1',$6,1,1)", source, actor.EnterpriseID, collector, generation, target, collection)
	service := telemetry.Service{Store: store}
	reader, err := service.ResourceQueryActor(ctx, actor.EnterpriseID, actor.SubjectID, actor.SubjectType, actor.AuthorizationVersion)
	if err != nil {
		t.Fatal(err)
	}
	includeSupported := false
	in := telemetry.ObservationCatalogInput{Scope: telemetry.QueryScope{Mode: "explicit", Resources: []resourceref.Ref{{Type: "monitoring_target", ID: target}}}, Signal: "metrics", Kind: "metrics", From: time.Now().Add(-time.Minute), To: time.Now(), Limit: 10, ObservedOnly: &includeSupported}
	recipes := []telemetry.ObservationRecipe{{ID: "mysql/test", Version: 1, SourceType: "mysql", IntegrationKey: "mysql", Metrics: []string{"a_observed", "b_supported"}}}
	empty := func() telemetry.DataCatalogResult {
		return telemetry.DataCatalogResult{Complete: true, Metrics: []telemetry.MetricDescriptor{}}
	}
	backend := &observationCatalogBudgetBackend{}
	service.Engine = backend
	backend.read = func(r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
		out := empty()
		out.Meta.ScannedBytes = 11
		if r.Metric == "" || r.Metric == "a_observed" {
			out.Metrics = []telemetry.MetricDescriptor{{Name: "a_observed", Type: "gauge", Unit: "1", SourceType: "mysql", SourceKeys: []string{source.String() + ":1"}}}
			// The observed phase already returned every variant. A targeted
			// existence lookup may stop after its first matching variant.
			if r.Metric != "" {
				out.Complete, out.HasMore = false, true
			}
		}
		return out, nil
	}
	out, err := service.ObservationCatalog(ctx, reader, in, recipes)
	if err != nil || len(out.Metrics) != 2 || !out.Metrics[0].Observed || out.Metrics[1].Observed || out.Metrics[1].Type != "unknown" || out.Metrics[1].Unit != "" || !out.Complete {
		t.Fatalf("observed and supported definitions merged/fabricated metadata: %+v %v", out, err)
	}
	if len(backend.requests) != 3 {
		t.Fatalf("unexpected targeted absence checks: %d", len(backend.requests))
	}
	for index, request := range backend.requests {
		if request.Budget.MaxScanBytes != telemetry.DefaultMaxScanBytes-int64(index*11) || len(request.SourceKeys) != 1 || request.SourceKeys[0] != source.String()+":1" {
			t.Fatal("candidate lookups reset scan budget or widened current authorized source", request)
		}
	}
	for _, failure := range []string{"backend_budget", "consumed_budget", "incomplete_empty"} {
		t.Run(failure, func(t *testing.T) {
			backend.requests = nil
			backend.read = func(r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
				out := empty()
				if r.Metric == "" {
					if failure == "consumed_budget" {
						out.Meta.ScannedBytes = telemetry.DefaultMaxScanBytes
					}
					return out, nil
				}
				if failure == "backend_budget" {
					return out, queryengine.ErrBudget
				}
				out.Complete = false
				return out, nil
			}
			out, err := service.ObservationCatalog(ctx, reader, in, recipes)
			if failure == "incomplete_empty" {
				if err != nil || len(out.Metrics) != 0 || out.Complete || !slices.Contains(out.Warnings, "SUPPORTED_METRIC_OBSERVATION_CHECK_INCOMPLETE") {
					t.Fatal("incomplete empty response fabricated supported-unobserved rows or claimed completeness", out, err)
				}
			} else if !errors.Is(err, queryengine.ErrBudget) && !errors.Is(err, telemetry.ErrQueryBudget) {
				t.Fatal("budget exhaustion became a successful empty catalog", out, err)
			}
			if failure == "consumed_budget" && len(backend.requests) != 1 {
				t.Fatal("data backend invoked after aggregate scan budget exhausted")
			}
		})
	}
	t.Log("Real PostgreSQL scope/source; controlled metered backend distinguishes observed/unobserved, cumulative budget, exhausted errors and incomplete absence")
}
