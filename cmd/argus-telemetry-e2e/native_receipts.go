//go:build m4e2e

package main

import (
	"context"
	"fmt"
	"io"
	"net"

	jaeger "github.com/uber/jaeger-client-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

// GO2Sky's official reporter owns the wire conversion, but closes asynchronously.
// This test-only transparent proxy waits for the actual upstream stream ACK.
// SkyWalking receiver v0.133 does not provide obsreport accepted-span counters.
type skyReceiptProxy struct {
	agentv3.UnimplementedTraceSegmentReportServiceServer
	ctx      context.Context
	upstream agentv3.TraceSegmentReportServiceClient
	result   chan error
}

func (p *skyReceiptProxy) Collect(down agentv3.TraceSegmentReportService_CollectServer) (err error) {
	defer func() {
		select {
		case p.result <- err:
		default:
		}
	}()
	up, err := p.upstream.Collect(p.ctx, grpc.WaitForReady(true))
	if err != nil {
		return err
	}
	count := 0
	for {
		segment, e := down.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		count += len(segment.Spans)
		if count > 32 {
			return fmt.Errorf("self-check span bound exceeded")
		}
		if e = up.Send(segment); e != nil {
			return e
		}
	}
	answer, err := up.CloseAndRecv()
	if err != nil {
		return err
	}
	if count != 4 {
		return fmt.Errorf("SkyWalking acknowledged %d spans, want 4", count)
	}
	return down.SendAndClose(answer)
}
func startSkyReceipt(ctx context.Context, endpoint string) (string, <-chan error, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = listener.Close()
		return "", nil, nil, err
	}
	result := make(chan error, 1)
	server := grpc.NewServer(grpc.MaxRecvMsgSize(1<<20), grpc.MaxConcurrentStreams(1))
	agentv3.RegisterTraceSegmentReportServiceServer(server, &skyReceiptProxy{ctx: ctx, upstream: agentv3.NewTraceSegmentReportServiceClient(conn), result: result})
	go func() { _ = server.Serve(listener) }()
	return listener.Addr().String(), result, func() { server.Stop(); _ = conn.Close(); _ = listener.Close() }, nil
}

// Use the SDK's own native Thrift transport and synchronous ACK accounting.
// The ordinary remote reporter only logs send errors, which is insufficient as
// a test assertion that all generated spans were actually accepted.
type jaegerReceiptReporter struct {
	transport jaeger.Transport
	count     int
	err       error
}

func (r *jaegerReceiptReporter) Report(span *jaeger.Span) {
	if r.err != nil {
		return
	}
	var count int
	count, r.err = r.transport.Append(span)
	if r.err == nil {
		r.count += count
		count, r.err = r.transport.Flush()
		if r.err == nil {
			r.count += count
		}
	}
}
func (r *jaegerReceiptReporter) Close() { _ = r.transport.Close() }
