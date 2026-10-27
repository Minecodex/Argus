package collectormanager_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestNativeIngestQuotaExplicitlyRejectsAndRecovers(t *testing.T) {
	var once sync.Once
	captured := make(chan *metricsrpc.ExportMetricsServiceRequest, 1)
	f := newNativeSDKProtocolPipelineWithFaults(t, "otlp", nativeSDKFaultOptions{MetricObserver: func(value *metricsrpc.ExportMetricsServiceRequest) {
		once.Do(func() { captured <- proto.Clone(value).(*metricsrpc.ExportMetricsServiceRequest) })
	}})
	exporter := plan8FaultExporter(t, f, f.sdkTLS())
	data := plan8FaultMetric(92, 1)
	if err := exporter.Export(f.ctx, &data); err != nil {
		t.Fatal("actual baseline SDK export failed", err)
	}
	var request *metricsrpc.ExportMetricsServiceRequest
	select {
	case request = <-captured:
	case <-time.After(15 * time.Second):
		t.Fatal("actual native request did not reach the real ingest")
	}
	certificate, err := tls.LoadX509KeyPair(filepath.Join(f.d.directory, "client.pem"), filepath.Join(f.d.directory, "client-key.pem"))
	if err != nil {
		t.Fatal("owned native client identity unavailable")
	}
	connection, err := grpc.NewClient(f.ingestQuotaAddress, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: f.certs.pool, Certificates: []tls.Certificate{certificate}, ServerName: "localhost"})))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := metricsrpc.NewMetricsServiceClient(connection)
	minute := time.Now().UTC().Truncate(time.Minute)
	keys := []string{"argus:telemetry:ingest:" + f.collector.String() + ":" + minute.Format("200601021504"), "argus:telemetry:ingest:" + f.collector.String() + ":" + minute.Add(time.Minute).Format("200601021504")}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = f.quotaRedis.Raw.Del(cleanup, keys...).Err()
	}()
	for _, key := range keys {
		if err = f.quotaRedis.Raw.Set(f.ctx, key, 1200, 2*time.Minute).Err(); err != nil {
			t.Fatal("owned quota state injection failed")
		}
	}
	_, err = client.Export(f.ctx, request)
	code := status.Code(err)
	retryAdvice, quotaReason := false, false
	for _, detail := range status.Convert(err).Details() {
		switch value := detail.(type) {
		case *errdetails.RetryInfo:
			retryAdvice = value.GetRetryDelay().AsDuration() > 0 && value.GetRetryDelay().AsDuration() <= time.Minute
		case *errdetails.ErrorInfo:
			quotaReason = value.Reason == "INGEST_RATE_LIMITED"
		}
	}
	proof := map[string]any{"passed": false, "scope": "actual_native_sdk_request_real_mtls_ingest_redis_quota_state_rejection_and_recovery", "controlled_quota_exhaustion_injected": true, "frozen_capacity_profile_pass_claimed": false, "expected_grpc_code": codes.ResourceExhausted.String(), "observed_grpc_code": code.String()}

	raw, _ := proto.Marshal(request)
	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: f.certs.pool, Certificates: []tls.Certificate{certificate}, ServerName: "localhost"}}}
	httpRequest, e := http.NewRequestWithContext(f.ctx, http.MethodPost, f.ingestQuotaHTTPAddress+"/v1/metrics", bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	httpRequest.Header.Set("Content-Type", "application/x-protobuf")
	response, e := httpClient.Do(httpRequest)
	httpLimited := false
	retrySeconds := 0
	if e == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
		_ = response.Body.Close()
		retrySeconds, _ = strconv.Atoi(response.Header.Get("Retry-After"))
		httpLimited = response.StatusCode == http.StatusTooManyRequests && retrySeconds > 0 && retrySeconds <= 60
	}
	proof["http_429_with_retry_after"] = httpLimited
	proof["http_retry_seconds"] = retrySeconds
	proof["retry_info_observed"] = retryAdvice
	proof["explicit_quota_reason_observed"] = quotaReason
	for _, key := range keys {
		if e := f.quotaRedis.Raw.Del(f.ctx, key).Err(); e != nil {
			t.Fatal("owned quota state recovery failed")
		}
	}
	_, recovered := client.Export(f.ctx, request)
	proof["actual_request_after_recovery_succeeded"] = recovered == nil
	if (code != codes.ResourceExhausted && code != codes.Unavailable) || !retryAdvice || !quotaReason || !httpLimited || recovered != nil {
		t.Errorf("quota requires explicit exhaustion and real recovery, status=%s recovered=%t", code.String(), recovered == nil)
	} else {
		proof["passed"] = true
	}
	if path := os.Getenv("ARGUS_PLAN8_QUOTA_EVIDENCE"); path != "" {
		body, _ := json.MarshalIndent(proof, "", "  ")
		if err = os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
