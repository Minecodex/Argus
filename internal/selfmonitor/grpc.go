package selfmonitor

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type metadataCarrier metadata.MD

func (m metadataCarrier) Get(k string) string {
	v := metadata.MD(m).Get(k)
	if len(v) > 0 {
		return v[0]
	}
	return ""
}
func (m metadataCarrier) Set(k, v string) { metadata.MD(m).Set(k, v) }
func (m metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
func rpcSpan(ctx context.Context, method string, kind trace.SpanKind) (context.Context, trace.Span) {
	service, name, _ := strings.Cut(strings.TrimPrefix(method, "/"), "/")
	return otel.Tracer("argus.grpc").Start(ctx, method, trace.WithSpanKind(kind), trace.WithAttributes(attribute.String("rpc.system", "grpc"), attribute.String("rpc.service", service), attribute.String("rpc.method", name)))
}
func endRPC(span trace.Span, err error) {
	span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(status.Code(err))))
	if err != nil {
		span.SetStatus(codes.Error, status.Code(err).String())
	}
	span.End()
}
func UnaryClient(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx, span := rpcSpan(ctx, method, trace.SpanKindClient)
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	if md == nil {
		md = metadata.MD{}
	}
	propagation.TraceContext{}.Inject(ctx, metadataCarrier(md))
	err := invoke(metadata.NewOutgoingContext(ctx, md), method, req, reply, cc, opts...)
	endRPC(span, err)
	return err
}
func UnaryServer(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = propagation.TraceContext{}.Extract(ctx, metadataCarrier(md))
	ctx, span := rpcSpan(ctx, info.FullMethod, trace.SpanKindServer)
	result, err := handler(ctx, req)
	endRPC(span, err)
	return result, err
}
