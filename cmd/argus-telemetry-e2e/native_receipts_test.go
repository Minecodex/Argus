//go:build m4e2e

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	commonv3 "skywalking.apache.org/repo/goapi/collect/common/v3"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

type skyAckServer struct {
	agentv3.UnimplementedTraceSegmentReportServiceServer
	fail bool
}

func (s skyAckServer) Collect(stream agentv3.TraceSegmentReportService_CollectServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if s.fail {
		return status.Error(codes.Unavailable, "receiver failed")
	}
	return stream.SendAndClose(&commonv3.Commands{})
}
func TestSkyReceiptWaitsForUpstreamConfirmation(t *testing.T) {
	for _, failure := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := grpc.NewServer()
		agentv3.RegisterTraceSegmentReportServiceServer(server, skyAckServer{fail: failure})
		go func() { _ = server.Serve(listener) }()
		address, receipt, stop, err := startSkyReceipt(ctx, listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		client, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		stream, err := agentv3.NewTraceSegmentReportServiceClient(client).Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = stream.Send(&agentv3.SegmentObject{Spans: []*agentv3.SpanObject{{}, {}, {}, {}}}); err != nil {
			t.Fatal(err)
		}
		_, _ = stream.CloseAndRecv()
		select {
		case got := <-receipt:
			if (got != nil) != failure {
				t.Fatalf("upstream ACK lost: %v", got)
			}
		case <-ctx.Done():
			t.Fatal("receipt timed out")
		}
		_ = client.Close()
		stop()
		server.Stop()
		_ = listener.Close()
		cancel()
	}
}

func TestJaegerReceiptCountsOnlyAcceptedNativePosts(t *testing.T) {
	for _, httpStatus := range []int{202, 503} {
		posts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			posts++
			if r.Header.Get("Content-Type") != "application/x-thrift" {
				t.Error("not native Jaeger Thrift")
			}
			w.WriteHeader(httpStatus)
		}))
		reporter := &jaegerReceiptReporter{transport: transport.NewHTTPTransport(server.URL)}
		tracer, closer := jaeger.NewTracer("test", jaeger.NewConstSampler(true), reporter)
		parent := tracer.StartSpan("observed request")
		child := tracer.StartSpan("outbound", opentracing.ChildOf(parent.Context()))
		child.Finish()
		parent.Finish()
		if httpStatus == 202 && (reporter.err != nil || reporter.count != 2 || posts != 2) {
			t.Fatalf("successful native posts not counted: %d %v", reporter.count, reporter.err)
		}
		if httpStatus == 503 && (reporter.err == nil || reporter.count != 0) {
			t.Fatal("failed native post claimed accepted")
		}
		_ = closer.Close()
		server.Close()
	}
}
