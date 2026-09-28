package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"time"

	commonv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/common/v1"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (server *QueryRPCServer) DiscoverData(ctx context.Context, request *telemetryv1.DiscoverDataRequest) (*telemetryv1.DiscoverDataResponse, error) {
	if !trustedRPCPeer(ctx) || server.Limiter == nil {
		return nil, status.Error(codes.Unauthenticated, "telemetry catalog mTLS identity required")
	}
	backend, ok := server.Backend.(DataCatalogBackend)
	if !ok {
		return nil, status.Error(codes.Unavailable, "telemetry catalog unavailable")
	}
	if request == nil || request.Scope == nil || request.From == nil || request.To == nil || !request.From.IsValid() || !request.To.IsValid() || len(request.FiltersJson) > 65536 || len(request.Scope.SourceKeys) == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid catalog request")
	}
	enterprise, resources, err := scopeFromProto(request.Scope, request.Signal)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid catalog scope")
	}
	budget, err := queryBudgetFromProto(request.Budget)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid catalog budget")
	}
	filters := []CatalogFilter{}
	if len(request.FiltersJson) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(request.FiltersJson))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&filters); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid catalog filters")
		}
		if decoder.Decode(new(any)) != io.EOF {
			return nil, status.Error(codes.InvalidArgument, "invalid catalog filters")
		}
	}
	if server.Readiness != nil {
		ready, e := server.Readiness.TelemetryTenantReady(ctx, enterprise)
		if e != nil || !ready {
			return nil, status.Error(codes.Unavailable, "telemetry tenant unavailable")
		}
	}
	release, err := server.Limiter.Acquire(ctx, enterprise, request.Scope.ScopeHash, budget.Timeout+time.Second*5)
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, "catalog concurrency unavailable")
	}
	defer release()
	started := time.Now()
	result, err := backend.DiscoverData(ctx, DataCatalogRequest{SubjectID: uuid.MustParse(request.Scope.SubjectId), SubjectType: request.Scope.SubjectType, EnterpriseID: enterprise, ResourceIDs: resources, SourceKeys: request.Scope.SourceKeys, AuthorizationVersion: int64(request.Scope.AuthorizationVersion), Signal: request.Signal, Kind: request.Kind, Metric: request.Metric, Field: request.Field, Search: request.Search, Cursor: request.Cursor, SelectedValues: request.SelectedValues, Filters: filters, From: request.From.AsTime(), To: request.To.AsTime(), Limit: int(request.Limit), Budget: budget})
	material, _ := json.Marshal(request)
	digest := sha256.Sum256(material)
	hash := hex.EncodeToString(digest[:])
	result.Meta.PlanHash = hash
	if server.Coordinator != nil && server.Coordinator.Audit != nil {
		_ = server.Coordinator.Audit.Record(ctx, queryengine.AuditEvent{SubjectID: uuid.MustParse(request.Scope.SubjectId), SubjectType: request.Scope.SubjectType, Language: queryengine.Language("catalog_" + request.Signal), EnterpriseID: enterprise, ResourceIDs: resources, AuthorizationVersion: int64(request.Scope.AuthorizationVersion), PlanHash: hash, ExpressionHash: hash, StartedAt: started, Elapsed: time.Since(started), Success: err == nil, Error: errorStringForCatalog(err), Meta: result.Meta})
	}
	if err != nil {
		if errors.Is(err, ErrQueryInvalid) {
			return nil, status.Error(codes.InvalidArgument, "QUERY_INVALID")
		}
		if errors.Is(err, ErrQueryBudget) || errors.Is(err, queryengine.ErrBudget) {
			return nil, status.Error(codes.ResourceExhausted, "QUERY_BUDGET_EXCEEDED")
		}
		return nil, status.Error(codes.Unavailable, "catalog discovery failed")
	}
	encoded, err := json.Marshal(result)
	if err != nil || int64(len(encoded)) > budget.MaxResultBytes {
		return nil, status.Error(codes.ResourceExhausted, "QUERY_BUDGET_EXCEEDED")
	}
	return &telemetryv1.DiscoverDataResponse{SchemaVersion: "argus.telemetry_catalog/v1", ResultJson: encoded}, nil
}

func errorStringForCatalog(err error) string {
	if err != nil {
		return "catalog_failed"
	}
	return ""
}

func (backend *GRPCQueryBackend) DiscoverData(ctx context.Context, input DataCatalogRequest) (DataCatalogResult, error) {
	if backend == nil || backend.connection == nil {
		return DataCatalogResult{}, ErrQueryBackend
	}
	resources := []*commonv1.ResourceRef{}
	for _, id := range input.ResourceIDs {
		resources = append(resources, &commonv1.ResourceRef{ResourceType: "host", ResourceId: id.String()})
	}
	scope := queryProtoScope(input.EnterpriseID, input.SubjectID, input.SubjectType, resources, input.ResourceIDs, input.AuthorizationVersion, input.Signal, input.SourceKeys)
	filters, err := json.Marshal(input.Filters)
	if err != nil {
		return DataCatalogResult{}, err
	}
	budget := &telemetryv1.QueryBudget{MaxScanBytes: uint64(input.Budget.MaxScanBytes), MaxRows: uint32(input.Budget.MaxRows), MaxResultBytes: uint64(input.Budget.MaxResultBytes), TimeoutMillis: uint32(input.Budget.Timeout.Milliseconds())}
	response, err := telemetryv1.NewTelemetryCatalogServiceClient(backend.connection).DiscoverData(ctx, &telemetryv1.DiscoverDataRequest{Scope: scope, Signal: input.Signal, Kind: input.Kind, Metric: input.Metric, Field: input.Field, Search: input.Search, Cursor: input.Cursor, SelectedValues: input.SelectedValues, FiltersJson: filters, From: timestamppb.New(input.From), To: timestamppb.New(input.To), Budget: budget, Limit: uint32(input.Limit)})
	if err != nil {
		return DataCatalogResult{}, engineRPCError(err)
	}
	if response == nil || response.SchemaVersion != "argus.telemetry_catalog/v1" {
		return DataCatalogResult{}, ErrQueryBackend
	}
	var result DataCatalogResult
	if err := json.Unmarshal(response.ResultJson, &result); err != nil {
		return result, ErrQueryBackend
	}
	return result, nil
}
