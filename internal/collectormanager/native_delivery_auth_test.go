package collectormanager_test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestNativeDeliveryControlRejectsForgedTargetsHeadersAndRevokedCertificate(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_PIPELINE") != "1" {
		t.Skip("requires owned native pipeline")
	}
	migrations := os.Getenv("ARGUS_PLAN8_DELIVERY_MIGRATIONS")
	if migrations == "" {
		migrations = filepath.Join("..", "..", "migrations", "postgresql")
	}
	isolated := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "delivery_auth", migrations)
	t.Setenv("ARGUS_MONITORING_TEST_DATABASE_URL", isolated.Pool.Config().ConnString())
	f := newNativeSDKProtocolPipelineWithFaults(t, "otlp", nativeSDKFaultOptions{Migrations: migrations})
	raw, err := os.ReadFile(filepath.Join(f.d.directory, "collector.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Extensions map[string]struct {
			Endpoint string `json:"delivery_endpoint"`
		} `json:"extensions"`
	}
	if json.Unmarshal(raw, &config) != nil {
		t.Fatal("native configuration unavailable")
	}
	endpoint, err := url.Parse(config.Extensions["argus_identity"].Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Port() == "" {
		t.Fatal("trusted delivery endpoint unavailable")
	}
	serverName := endpoint.Hostname()
	endpoint.Host = "127.0.0.1:" + endpoint.Port()
	certificate, err := tls.LoadX509KeyPair(filepath.Join(f.d.directory, "client.pem"), filepath.Join(f.d.directory, "client-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: f.certs.pool, Certificates: []tls.Certificate{certificate}, ServerName: serverName, MinVersion: tls.VersionTLS13}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	at := time.Now().UTC()
	report := telemetry.CollectorDeliveryReport{SchemaVersion: "argus.collector_delivery/v1", InstanceID: uuid.NewString(), Sequence: 1, ObservedSince: at, ObservedAt: at, RetryLimitSeconds: 300, Attribution: "collector_shared_pipeline_exact_targets_unknown"}
	post := func(body any, headers map[string]string) int {
		t.Helper()
		value, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(f.ctx, http.MethodPost, endpoint.String(), bytes.NewReader(value))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("actual mTLS delivery request failed", err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return response.StatusCode
	}
	if got := post(report, nil); got != http.StatusNoContent {
		t.Fatal("valid collector control report rejected", got)
	}
	if got := post(report, nil); got != http.StatusNoContent {
		t.Fatal("idempotent report rejected", got)
	}
	forged := map[string]any{}
	value, _ := json.Marshal(report)
	_ = json.Unmarshal(value, &forged)
	forged["collector_id"] = uuid.NewString()
	forged["resource_id"] = uuid.NewString()
	if got := post(forged, nil); got != http.StatusBadRequest {
		t.Fatal("forged target fields accepted", got)
	}
	for _, headers := range []map[string]string{{"Authorization": "Bearer never-forward-this-key"}, {"Cookie": "session=never-forward-this-session"}} {
		if got := post(report, headers); got != http.StatusUnauthorized {
			t.Fatal("platform credential header accepted on Collector control route", got)
		}
	}
	bad := report
	bad.SchemaVersion = "argus.collector_delivery/v0"
	if got := post(bad, nil); got != http.StatusBadRequest {
		t.Fatal("unsupported report identity accepted", got)
	}
	bad = report
	bad.RetryLimitSeconds = 0
	if got := post(bad, nil); got != http.StatusBadRequest {
		t.Fatal("unbounded retry report accepted", got)
	}
	f.exec("UPDATE telemetry_certificates SET revoked_at=now() WHERE collector_id=$1 AND serial_number=$2", f.collector, f.certs.serial)
	if got := post(report, nil); got != http.StatusUnauthorized {
		t.Fatal("revoked certificate remained usable over established TLS", got)
	}
	noCertificate := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: f.certs.pool, ServerName: serverName, MinVersion: tls.VersionTLS13}}
	defer noCertificate.CloseIdleConnections()
	unauthenticated := &http.Client{Transport: noCertificate, Timeout: 5 * time.Second}
	response, err := unauthenticated.Post(endpoint.String(), "application/json", bytes.NewReader(value))
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode == http.StatusNoContent {
			t.Fatal("missing mTLS certificate accepted")
		}
	}
}
