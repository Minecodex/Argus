package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type metricMetadataRPCEngine struct{ calls int }

func (e *metricMetadataRPCEngine) Execute(_ context.Context, request queryengine.Request) (queryengine.Result, error) {
	e.calls++
	return queryengine.Result{Language: queryengine.LanguagePromQL, ResultType: "matrix", Data: []any{}, Meta: queryengine.QueryMeta{
		InputMetricsOrigin: metricmeta.Origin, InputMetricsComplete: true, ResultIsOriginalMetric: true,
		InputMetrics: []metricmeta.Descriptor{{Name: "network_bytes", Type: "counter", StorageType: "sum", Temporality: "cumulative", Monotonic: true, Unit: "By", SourceKey: request.Scope.SourceKeys[0]}},
	}}, nil
}

func TestInputMetricMetadataSurvivesSerializedRPCCacheAndProtocolProjection(t *testing.T) {
	engine, limiter := &metricMetadataRPCEngine{}, &cacheRPCLimiter{}
	server := &QueryRPCServer{Limiter: limiter, Coordinator: &queryengine.Coordinator{PromQL: engine, Cache: queryengine.NewResultCache(1<<20, time.Minute)}}
	backend := GRPCQueryBackend{client: cacheRPCClient{server: server}}
	r := queryengine.Request{CacheNamespace: strings.Repeat("a", 64), Language: queryengine.LanguagePromQL, Expression: "network_bytes", Start: time.Unix(10, 0), End: time.Unix(30, 0), Step: time.Second,
		Scope: queryengine.Scope{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", ResourceIDs: []uuid.UUID{uuid.New()}, SourceKeys: []string{uuid.NewString() + ":2"}, AuthorizationVersion: 1}}
	r.Scope.Resources = testResourceRefs(r.Scope.ResourceIDs[0])
	r.Scope.Signature = testScopeSignature
	for attempt := 0; attempt < 2; attempt++ {
		result, err := backend.ExecuteEngineQuery(t.Context(), r)
		if err != nil {
			t.Fatal(err)
		}
		if result.Meta.InputMetricsOrigin != metricmeta.Origin || !result.Meta.InputMetricsComplete || !result.Meta.ResultIsOriginalMetric || len(result.Meta.InputMetrics) != 1 || result.Meta.InputMetrics[0].Type != "counter" || result.Meta.InputMetrics[0].Unit != "By" || result.Meta.InputMetrics[0].SourceKey != r.Scope.SourceKeys[0] || result.Meta.CacheHit != (attempt == 1) {
			t.Fatal("RPC/cache lost authoritative input metadata", result.Meta)
		}
		output := protocolQueryOutput(result)
		meta := output["argus_meta"].(map[string]any)
		if meta["input_metrics_origin"] != metricmeta.Origin || meta["input_metrics_complete"] != true || len(meta["input_metrics"].([]metricmeta.Descriptor)) != 1 {
			t.Fatal("native query file projection lost source metadata")
		}
	}
	if engine.calls != 1 || limiter.calls != 2 {
		t.Fatal("metadata bypassed normal cache/limiter rules", engine.calls, limiter.calls)
	}
}
