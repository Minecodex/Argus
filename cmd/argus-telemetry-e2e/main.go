//go:build m4e2e

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	endpoint := flag.String("endpoint", "argus-otelcol-gateway.argus-telemetry.svc.cluster.local:4317", "OTLP gRPC endpoint")
	resourceID := flag.String("resource-id", "e2e-resource", "resource marker")
	collectorID := flag.String("collector-id", "", "trusted Collector UUID for Kafka injection")
	enterpriseID := flag.String("enterprise-id", "", "trusted Enterprise UUID for Kafka injection")
	marker := flag.String("marker", "", "suffix used to distinguish recovery records")
	logBody := flag.String("log-body", "", "explicit log body used by authorization and redaction tests")
	kafkaBrokers := flag.String("kafka-brokers", "", "comma-separated Kafka brokers for permanent-record injection")
	kafkaUsername := flag.String("kafka-username", "", "Kafka SCRAM username")
	kafkaPassword := flag.String("kafka-password", "", "Kafka SCRAM password")
	tlsCA := flag.String("tls-ca", "", "trusted OTLP server CA path")
	tlsCertificate := flag.String("tls-cert", "", "OTLP client certificate path")
	tlsKey := flag.String("tls-key", "", "OTLP client private key path")
	tlsServerName := flag.String("tls-server-name", "", "expected OTLP server name")
	selfcheckURL := flag.String("native-selfcheck-url", "", "exercise native SkyWalking/Jaeger SDKs against the local Argus HTTP service")
	sharedService := flag.String("native-service-name", "", "optional common service name for native source isolation checks")
	traceRole := flag.String("planv2-trace-role", "", "frontend or backend trace boundary fixture")
	traceTime := flag.String("planv2-trace-time", "", "shared RFC3339Nano event time for out-of-order spans")
	traceGeneration := flag.String("planv2-trace-generation", "old", "fixture body distinguishing installation generations")
	alertValue := flag.Float64("plan6-alert-value", math.NaN(), "real alert CPU fixture gauge; memory is five points lower")
	memoryValue := flag.Float64("plan6-memory-value", math.NaN(), "optional independent memory gauge for real inhibition recovery")
	memberSplit := flag.Bool("plan6-member-split", false, "emit additional gauges with distinct member labels")
	templateRED := flag.Bool("template-red-fixture", false, "emit isolated tool-template RED acceptance observations")
	flag.Parse()
	if *kafkaPassword == "" {
		*kafkaPassword = os.Getenv("ARGUS_E2E_KAFKA_PASSWORD")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *selfcheckURL != "" {
		if err := nativeNamedSelfcheck(ctx, *selfcheckURL, *sharedService); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *kafkaBrokers != "" {
		producePermanentRecord(ctx, strings.Split(*kafkaBrokers, ","), *kafkaUsername, *kafkaPassword, *enterpriseID, *resourceID, *collectorID)
		return
	}
	transport := credentials.TransportCredentials(insecure.NewCredentials())
	if *tlsCA != "" || *tlsCertificate != "" || *tlsKey != "" || *tlsServerName != "" {
		transport = telemetryTransportCredentials(*tlsCA, *tlsCertificate, *tlsKey, *tlsServerName)
	}
	if *traceRole != "" {
		at, err := time.Parse(time.RFC3339Nano, *traceTime)
		if err == nil {
			err = emitPlanV2Trace(ctx, *endpoint, transport, *traceRole, *traceGeneration, at)
		}
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if *templateRED {
		if err := emitTemplateRED(ctx, *endpoint, transport, *resourceID); err != nil {
			log.Fatal(err)
		}
		return
	}
	now := uint64(time.Now().UTC().UnixNano())
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
		{Key: "service.name", Value: stringValue("argus-m7-e2e")},
		{Key: "argus.enterprise.id", Value: stringValue("forged-enterprise")},
		{Key: "argus.resource.id", Value: stringValue(*resourceID)},
	}}
	suffix := ""
	if *marker != "" {
		suffix = "." + *marker
	}
	// Use an independent authenticated RPC connection for each signal. This
	// keeps the gateway's per-RPC identity context isolated while still
	// exercising the same leaf certificate and route for all three signals.
	for _, export := range []func(grpc.ClientConnInterface) error{
		func(connection grpc.ClientConnInterface) error {
			request := metricsRequest(resource, now, suffix)
			if !math.IsNaN(*alertValue) {
				gauges := plan6Metrics(now, *alertValue)
				if !math.IsNaN(*memoryValue) {
					gauges[1].GetGauge().DataPoints[0].Value = &metricspb.NumberDataPoint_AsDouble{AsDouble: *memoryValue}
				}
				if *memberSplit {
					for index, name := range []string{"argus_plan6_member_cpu_percent", "argus_plan6_member_memory_percent"} {
						gauges = append(gauges, &metricspb.Metric{Name: name, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: now, Attributes: []*commonpb.KeyValue{{Key: "plan6_member", Value: stringValue([]string{"cpu", "memory"}[index])}}, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 95}}}}}})
					}
				}
				request.ResourceMetrics[0].ScopeMetrics[0].Metrics = append(request.ResourceMetrics[0].ScopeMetrics[0].Metrics, gauges...)
			}
			if suffix == ".planv2" {
				request.ResourceMetrics[0].ScopeMetrics[0].Metrics = append(request.ResourceMetrics[0].ScopeMetrics[0].Metrics, planV2SelectionMetric(now))
			}
			_, err := collectmetrics.NewMetricsServiceClient(connection).Export(ctx, request)
			return err
		},
		func(connection grpc.ClientConnInterface) error {
			request := logsRequest(resource, now, suffix, *logBody)
			if suffix == ".planv2" {
				request.ResourceLogs[0].ScopeLogs[0].LogRecords = append(request.ResourceLogs[0].ScopeLogs[0].LogRecords, planV2SelectionLogs(now)...)
			}
			_, err := collectlogs.NewLogsServiceClient(connection).Export(ctx, request)
			return err
		},
		func(connection grpc.ClientConnInterface) error {
			_, err := collecttraces.NewTraceServiceClient(connection).Export(ctx, tracesRequest(resource, now, suffix))
			return err
		},
	} {
		if err := exportWithRetry(ctx, *endpoint, transport, 500*time.Millisecond, export); err != nil {
			log.Fatal(err)
		}
	}
}

func exportWithRetry(ctx context.Context, endpoint string, transport credentials.TransportCredentials, retryDelay time.Duration, export func(grpc.ClientConnInterface) error) error {
	var lastErr error
	for {
		connection, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(transport))
		if err == nil {
			err = export(connection)
			_ = connection.Close()
		}
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(retryDelay):
		}
	}
}

func telemetryTransportCredentials(caPath, certificatePath, keyPath, serverName string) credentials.TransportCredentials {
	if caPath == "" || certificatePath == "" || keyPath == "" || serverName == "" {
		log.Fatal("complete telemetry mTLS configuration is required")
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		log.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		log.Fatal("telemetry CA bundle is invalid")
	}
	certificate, err := tls.LoadX509KeyPair(certificatePath, keyPath)
	if err != nil {
		log.Fatal(err)
	}
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}, ServerName: serverName})
}

func metricsRequest(resource *resourcepb.Resource, now uint64, suffix string) *collectmetrics.ExportMetricsServiceRequest {
	histogramSum := 7.0
	classicSum := 16.0
	return &collectmetrics.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: resource,
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Metrics: []*metricspb.Metric{
					{Name: "system.cpu.utilization", Unit: "1", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: now, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.42}}},
					}}},
					{Name: "argus.m7.e2e.gauge" + suffix, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: now, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 7}}},
					}}},
					{Name: "argus.m7.e2e.native.histogram" + suffix, Data: &metricspb.Metric_ExponentialHistogram{ExponentialHistogram: &metricspb.ExponentialHistogram{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						DataPoints: []*metricspb.ExponentialHistogramDataPoint{{TimeUnixNano: now, Count: 3, Sum: &histogramSum, Scale: 1, ZeroCount: 1,
							Positive: &metricspb.ExponentialHistogramDataPoint_Buckets{Offset: 0, BucketCounts: []uint64{1, 1}}}},
					}}},
					{Name: "argus.m7.e2e.summary" + suffix, Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{DataPoints: []*metricspb.SummaryDataPoint{{
						TimeUnixNano: now, Count: 2, Sum: 7, QuantileValues: []*metricspb.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 3}, {Quantile: 0.9, Value: 4}},
					}}}}},
					{Name: "argus.planv2.latency", Unit: "ms", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.HistogramDataPoint{{TimeUnixNano: now, Count: 6, Sum: &classicSum, ExplicitBounds: []float64{1, 5}, BucketCounts: []uint64{2, 3, 1}}}}}},
				},
			}},
		}},
	}
}

func logsRequest(resource *resourcepb.Resource, now uint64, suffix, explicitBody string) *collectlogs.ExportLogsServiceRequest {
	body := "argus m7 e2e log" + suffix
	if explicitBody != "" {
		body = explicitBody
	}
	return &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource,
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: now, SeverityText: "INFO", Body: stringValue(body)}}}}}}}
}

func tracesRequest(resource *resourcepb.Resource, now uint64, suffix string) *collecttraces.ExportTraceServiceRequest {
	return &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: resource,
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: []byte("0123456789abcdef"), SpanId: []byte("12345678"), Name: "argus-m7-e2e" + suffix,
			Kind: tracepb.Span_SPAN_KIND_INTERNAL, StartTimeUnixNano: now, EndTimeUnixNano: now + uint64(time.Millisecond)}}}}}}}
}

func producePermanentRecord(ctx context.Context, brokers []string, username, password, enterpriseID, resourceID, collectorID string) {
	if len(brokers) == 0 || username == "" || password == "" || enterpriseID == "" || resourceID == "" || collectorID == "" {
		log.Fatal("Kafka injection requires brokers, credentials, and trusted identity")
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.SASL(scram.Auth{User: username, Pass: password}.AsSha512Mechanism()),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	record := &kgo.Record{Topic: "otlp-metrics", Value: []byte("not-an-otlp-protobuf"), Headers: []kgo.RecordHeader{
		{Key: "argus-enterprise-id", Value: []byte(enterpriseID)},
		{Key: "argus-resource-id", Value: []byte(resourceID)},
		{Key: "argus-collector-id", Value: []byte(collectorID)},
		{Key: "argus-resource-type", Value: []byte("kubernetes_cluster")},
	}}
	if err = client.ProduceSync(ctx, record).FirstErr(); err != nil {
		log.Fatal(err)
	}
}

func stringValue(value string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
}

func plan6Metrics(now uint64, value float64) []*metricspb.Metric {
	out := []*metricspb.Metric{}
	for _, item := range []struct {
		name  string
		value float64
	}{{"argus_plan6_cpu_percent", value}, {"argus_plan6_memory_percent", value - 5}} {
		out = append(out, &metricspb.Metric{Name: item.name, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: now, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: item.value}}}}}})
	}
	points := []*metricspb.NumberDataPoint{}
	for index, sample := range []float64{100, 110, 5, 15} {
		points = append(points, &metricspb.NumberDataPoint{StartTimeUnixNano: now - uint64(time.Minute), TimeUnixNano: now - uint64(time.Duration(3-index)*10*time.Second), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: sample}})
	}
	out = append(out, &metricspb.Metric{Name: "argus.plan6.counter.total", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true, DataPoints: points}}})
	return out
}
