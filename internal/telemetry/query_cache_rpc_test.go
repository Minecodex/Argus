package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
)

type cacheRPCClient struct {
	telemetryv1.TelemetryQueryServiceClient
	server *QueryRPCServer
}

func (client cacheRPCClient) ExecuteQueryV2(ctx context.Context, request *telemetryv1.ExecuteQueryV2Request, _ ...grpc.CallOption) (*telemetryv1.ExecuteQueryV2Response, error) {
	encoded, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	var wire telemetryv1.ExecuteQueryV2Request
	if err = proto.Unmarshal(encoded, &wire); err != nil {
		return nil, err
	}
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}}})
	response, err := client.server.ExecuteQueryV2(ctx, &wire)
	if err != nil {
		return nil, err
	}
	encoded, err = proto.Marshal(response)
	if err != nil {
		return nil, err
	}
	var result telemetryv1.ExecuteQueryV2Response
	err = proto.Unmarshal(encoded, &result)
	return &result, err
}

type cacheRPCLimiter struct{ calls int }

func (limiter *cacheRPCLimiter) Acquire(context.Context, uuid.UUID, string, time.Duration) (func(), error) {
	limiter.calls++
	return func() {}, nil
}

type cacheRPCEngine struct{ calls int }

func (engine *cacheRPCEngine) Execute(context.Context, queryengine.Request) (queryengine.Result, error) {
	engine.calls++
	sample := time.Unix(20, 123).UTC()
	return queryengine.Result{Language: queryengine.LanguageKQL, ResultType: "log_entries", Data: []map[string]any{{"body": "evidence"}}, Meta: queryengine.QueryMeta{LatestSampleAt: &sample}}, nil
}
func TestQueryCacheAndFreshnessSurviveRPCWithLimiterOnEveryHit(t *testing.T) {
	engine, limiter := &cacheRPCEngine{}, &cacheRPCLimiter{}
	server := &QueryRPCServer{Limiter: limiter, Coordinator: &queryengine.Coordinator{KQL: engine, Cache: queryengine.NewResultCache(1<<20, time.Minute)}}
	backend := GRPCQueryBackend{client: cacheRPCClient{server: server}}
	r := queryengine.Request{CacheNamespace: strings.Repeat("a", 64), Language: queryengine.LanguageKQL, Expression: "*", Start: time.Unix(10, 0), End: time.Unix(30, 0), Scope: queryengine.Scope{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", ResourceIDs: []uuid.UUID{uuid.New()}, SourceKeys: []string{uuid.NewString() + ":1"}, AuthorizationVersion: 1}}
	first, err := backend.ExecuteEngineQuery(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.ExecuteEngineQuery(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if engine.calls != 1 || limiter.calls != 2 || first.Meta.CacheHit || !second.Meta.CacheHit {
		t.Fatalf("RPC cache/limiter not respected: engine=%d limiter=%d", engine.calls, limiter.calls)
	}
	if second.Meta.LatestSampleAt == nil || !second.Meta.LatestSampleAt.Equal(time.Unix(20, 123)) || second.Meta.QueryCompletedAt == nil || !first.Meta.QueryCompletedAt.Equal(*second.Meta.QueryCompletedAt) || second.Meta.SampleTimeBasis != "observed_query_samples" || second.Meta.IngestionStatus != "unknown" {
		t.Fatalf("RPC dropped freshness fields: %+v", second.Meta)
	}
}
