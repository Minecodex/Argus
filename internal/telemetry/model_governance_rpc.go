package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type ModelGovernanceRPC struct {
	telemetryv1.UnimplementedModelGovernanceServiceServer
	Service modelmetrics.Service
	Limiter QueryConcurrencyLimiter
}

func RegisterModelGovernanceRPC(server *grpc.Server, service modelmetrics.Service, limiter QueryConcurrencyLimiter) {
	telemetryv1.RegisterModelGovernanceServiceServer(server, &ModelGovernanceRPC{Service: service, Limiter: limiter})
}
func (s *ModelGovernanceRPC) QueryModelGovernance(ctx context.Context, request *telemetryv1.QueryModelGovernanceRequest) (*telemetryv1.QueryModelGovernanceResponse, error) {
	if !modelGovernancePeer(ctx) {
		return nil, status.Error(codes.Unauthenticated, "model metrics require control-plane mTLS")
	}
	if request == nil || request.SchemaVersion != modelmetrics.ProtocolVersion || len(request.RequestJson) > 65536 {
		return nil, status.Error(codes.InvalidArgument, "invalid model metric request")
	}
	var input modelmetrics.Request
	decoder := json.NewDecoder(bytes.NewReader(request.RequestJson))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid model metric request")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, status.Error(codes.InvalidArgument, "invalid model metric request")
	}
	if _, err := modelmetrics.Resolve(ctx, s.Service.Store, input); err != nil {
		return nil, modelMetricStatus(err)
	}
	if s.Limiter == nil {
		return nil, status.Error(codes.Unavailable, "model metric limiter unavailable")
	}
	release, err := s.Limiter.Acquire(ctx, input.Actor.EnterpriseID, "model-governance", 35*time.Second)
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, "model metric concurrency exceeded")
	}
	defer release()
	result, err := s.Service.QueryModelGovernance(ctx, input)
	if err != nil {
		return nil, modelMetricStatus(err)
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 8<<20 {
		return nil, status.Error(codes.ResourceExhausted, "model metric result exceeds budget")
	}
	return &telemetryv1.QueryModelGovernanceResponse{SchemaVersion: modelmetrics.ProtocolVersion, ResultJson: raw}, nil
}

func modelGovernancePeer(ctx context.Context) bool {
	if !trustedRPCPeer(ctx) {
		return false
	}
	value, _ := peer.FromContext(ctx)
	info := value.AuthInfo.(credentials.TLSInfo)
	for _, uri := range info.State.PeerCertificates[0].URIs {
		if uri.String() == "spiffe://argus.io/services/server/telemetry-client" {
			return true
		}
	}
	return false
}
func modelMetricStatus(err error) error {
	switch {
	case errors.Is(err, modelmetrics.ErrDenied):
		return status.Error(codes.PermissionDenied, "model governance denied")
	case errors.Is(err, modelmetrics.ErrInvalid):
		return status.Error(codes.InvalidArgument, "model governance invalid")
	case errors.Is(err, modelmetrics.ErrBudget):
		return status.Error(codes.ResourceExhausted, "model governance budget exceeded")
	default:
		return status.Error(codes.Unavailable, "model governance unavailable")
	}
}
func (b *GRPCQueryBackend) QueryModelGovernance(ctx context.Context, input modelmetrics.Request) (modelmetrics.Response, error) {
	if b == nil || b.connection == nil {
		return modelmetrics.Response{}, modelmetrics.ErrUnavailable
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return modelmetrics.Response{}, err
	}
	result, err := telemetryv1.NewModelGovernanceServiceClient(b.connection).QueryModelGovernance(ctx, &telemetryv1.QueryModelGovernanceRequest{SchemaVersion: modelmetrics.ProtocolVersion, RequestJson: raw}, grpc.MaxCallRecvMsgSize(8<<20))
	if err != nil {
		switch status.Code(err) {
		case codes.PermissionDenied:
			return modelmetrics.Response{}, modelmetrics.ErrDenied
		case codes.InvalidArgument:
			return modelmetrics.Response{}, modelmetrics.ErrInvalid
		case codes.ResourceExhausted:
			return modelmetrics.Response{}, modelmetrics.ErrBudget
		}
		return modelmetrics.Response{}, modelmetrics.ErrUnavailable
	}
	if result.SchemaVersion != modelmetrics.ProtocolVersion {
		return modelmetrics.Response{}, modelmetrics.ErrUnavailable
	}
	var output modelmetrics.Response
	err = json.Unmarshal(result.ResultJson, &output)
	return output, err
}
