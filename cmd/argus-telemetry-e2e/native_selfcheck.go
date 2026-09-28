//go:build m4e2e

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/transport"
)

// These native SDK spans measure real read-only requests to Argus. They are
// explicitly named self-checks, not fabricated production service statistics.
func nativeSelfcheck(ctx context.Context, baseURL string) error {
	return nativeNamedSelfcheck(ctx, baseURL, "")
}

func nativeNamedSelfcheck(ctx context.Context, baseURL, commonService string) error {
	for _, endpoint := range []string{"127.0.0.1:11800", "127.0.0.1:14268"} {
		for {
			conn, e := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", endpoint)
			if e == nil {
				_ = conn.Close()
				break
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("native receiver %s not ready: %w", endpoint, ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	proxy, receipt, cleanup, err := startSkyReceipt(ctx, "127.0.0.1:11800")
	if err != nil {
		return err
	}
	defer cleanup()
	skyReporter, err := reporter.NewGRPCReporter(proxy, reporter.WithCheckInterval(-1), reporter.WithCDS(0), reporter.WithMaxSendQueueSize(128))
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			skyReporter.Close()
		}
	}()
	skyName, jaegerName := "argus-selfcheck-skywalking", "argus-selfcheck-jaeger"
	if commonService != "" {
		skyName, jaegerName = commonService, commonService
	}
	sky, err := go2sky.NewTracer(skyName, go2sky.WithReporter(skyReporter), go2sky.WithInstance("native-sdk-selfcheck"), go2sky.WithSampler(1))
	if err != nil {
		return err
	}
	jaegerReporter := &jaegerReceiptReporter{transport: transport.NewHTTPTransport("http://127.0.0.1:14268/api/traces", transport.HTTPTimeout(3*time.Second))}
	jt, closer := jaeger.NewTracer(jaegerName, jaeger.NewConstSampler(true), jaegerReporter, jaeger.TracerOptions.Tag("service.instance.id", "native-sdk-selfcheck"))
	defer closer.Close()
	for _, path := range []string{"/", "/argus-selfcheck-not-found"} {
		root, childCtx, e := sky.CreateEntrySpan(ctx, "selfcheck GET "+path, func(string) (string, error) { return "", nil })
		if e != nil {
			return e
		}
		request, _ := http.NewRequestWithContext(childCtx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+path, nil)
		exit, e := sky.CreateExitSpan(childCtx, "GET "+path, request.URL.Host, func(k, v string) error { request.Header.Set(k, v); return nil })
		if e != nil {
			root.End()
			return e
		}
		code, callErr := selfcheckRequest(client, request)
		exit.Tag(go2sky.Tag("http.status_code"), strconv.Itoa(code))
		root.Tag(go2sky.Tag("argus.selfcheck"), "true")
		if code >= 400 || callErr != nil {
			root.Error(time.Now(), "event", "HTTP check failed")
			exit.Error(time.Now(), "event", "HTTP check failed")
		}
		exit.End()
		root.End()
		if callErr != nil {
			return callErr
		}

		parent := jt.StartSpan("selfcheck GET " + path)
		ext.SpanKindRPCServer.Set(parent)
		parent.SetTag("argus.selfcheck", true)
		child := jt.StartSpan("GET "+path, opentracing.ChildOf(parent.Context()))
		ext.SpanKindRPCClient.Set(child)
		request, _ = http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+path, nil)
		if e = jt.Inject(child.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(request.Header)); e != nil {
			return e
		}
		code, callErr = selfcheckRequest(client, request)
		ext.HTTPStatusCode.Set(child, uint16(code))
		if code >= 400 || callErr != nil {
			ext.Error.Set(parent, true)
			ext.Error.Set(child, true)
			child.LogKV("event", "HTTP check failed")
		}
		child.Finish()
		parent.Finish()
		if callErr != nil {
			return callErr
		}
	}
	closed = true
	skyReporter.Close()
	if jaegerReporter.err != nil {
		return fmt.Errorf("Jaeger native receipt: %w", jaegerReporter.err)
	}
	if jaegerReporter.count != 4 {
		return fmt.Errorf("Jaeger acknowledged %d spans, want 4", jaegerReporter.count)
	}
	select {
	case err := <-receipt:
		return err
	case <-ctx.Done():
		return fmt.Errorf("SkyWalking native receipt: %w", ctx.Err())
	}
}

func selfcheckRequest(client *http.Client, request *http.Request) (int, error) {
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, err
}
