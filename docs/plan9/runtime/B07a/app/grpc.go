package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	logpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func grpcExporters(ctx context.Context, endpoint string, secure *tls.Config, headers map[string]string) (sdktrace.SpanExporter, sdklog.Exporter, sdkmetric.Exporter, func() error, error) {
	conn, err := dialOTLPGRPC(endpoint, secure)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	traces, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn), otlptracegrpc.WithHeaders(headers))
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, conn.Close())
	}
	logs, metrics, err := grpcLogMetricExporters(ctx, conn, headers)
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, traces.Shutdown(ctx), conn.Close())
	}
	return traces, logs, metrics, conn.Close, nil
}

func dialOTLPGRPC(endpoint string, secure *tls.Config) (*grpc.ClientConn, error) {
	var acknowledged sync.Map
	return grpc.NewClient(endpoint, grpc.WithTransportCredentials(credentials.NewTLS(secure)), grpc.WithUnaryInterceptor(
		func(ctx context.Context, method string, request, response any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			var remote peer.Peer
			if err := invoke(ctx, method, request, response, cc, append(opts, grpc.Peer(&remote))...); err != nil {
				return err
			}
			// Record only the first successful, fully accepted Export for each signal.
			var rejected int64
			switch value := response.(type) {
			case *tracepb.ExportTraceServiceResponse:
				rejected = value.GetPartialSuccess().GetRejectedSpans()
			case *metricpb.ExportMetricsServiceResponse:
				rejected = value.GetPartialSuccess().GetRejectedDataPoints()
			case *logpb.ExportLogsServiceResponse:
				rejected = value.GetPartialSuccess().GetRejectedLogRecords()
			default:
				return nil
			}
			if info, ok := remote.AuthInfo.(credentials.TLSInfo); ok && rejected == 0 {
				if _, seen := acknowledged.LoadOrStore(method, true); !seen {
					proof, _ := json.Marshal(map[string]any{"rpc": method, "tls_version": info.State.Version, "alpn": info.State.NegotiatedProtocol, "verified_chains": len(info.State.VerifiedChains), "rejected": rejected})
					fmt.Println("otlp_grpc_ack " + string(proof))
				}
			}
			return nil
		}))
}

func grpcLogMetricExporters(ctx context.Context, conn *grpc.ClientConn, headers map[string]string) (sdklog.Exporter, sdkmetric.Exporter, error) {
	logs, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn), otlploggrpc.WithHeaders(headers))
	if err != nil {
		return nil, nil, err
	}
	metrics, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn), otlpmetricgrpc.WithHeaders(headers))
	if err != nil {
		return nil, nil, errors.Join(err, logs.Shutdown(ctx))
	}
	return logs, metrics, nil
}
