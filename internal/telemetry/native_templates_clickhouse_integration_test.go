package telemetry

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

type nativeTemplateSourceControl struct{ sourceTestControl }

// Existing capture gates have controlled source admission and cover Cluster
// views only. They deliberately grant no Namespace row authority; the separate
// Namespace identity/query gates exercise that additional boundary.
func (c nativeTemplateSourceControl) ResolveMetricNamespace(context.Context, TrustedIdentity, SourceIdentity, string, string) (string, error) {
	return "", nil
}

func (c nativeTemplateSourceControl) ResolveMetricObjectNamespace(context.Context, TrustedIdentity, SourceIdentity, string, []MetricNamespaceObject) (string, error) {
	return "", nil // This historical fixture deliberately grants Cluster scope only.
}

func (c nativeTemplateSourceControl) ResolveSource(ctx context.Context, identity TrustedIdentity, id uuid.UUID, revision int64, signal string) (SourceIdentity, error) {
	if signal != "metrics" {
		return SourceIdentity{}, errIngestRejected
	}
	return c.sourceTestControl.ResolveSource(ctx, identity, id, revision, "logs")
}

// This gate uses unmodified OTLP captured from real native receivers. It covers
// canonicalization, the real Writer and ClickHouse PromQL expressions used by
// the locked templates. Network/PKI/Kafka admission and the deployed browser
// remain separate gates; this fixture does not claim platform E2E acceptance.
func TestClickHouseNativeTemplateCapturedMetrics(t *testing.T) {
	testCapturedNativeTemplates(t, 4, 22)
}

func TestClickHousePostgresProbeTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_POSTGRES_PROBE_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native PostgreSQL and probe samples")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 4, 14)
}

func TestClickHouseCacheWebTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_CACHE_WEB_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native Memcached/Apache/HAProxy capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 3, 18)
}

func TestClickHouseBrokerCoordinationTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_BROKER_COORDINATION_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native RabbitMQ/ZooKeeper capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 2, 12)
}

func TestClickHouseNetworkProbeTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_NETWORK_PROBE_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native DNS/ICMP capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 3, 9)
}

func TestClickHouseKafkaTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_KAFKA_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native Kafka capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 1, 8)
}

func TestClickHouseRemainingDatabaseTemplateCapturedMetrics(t *testing.T) {
	dir := os.Getenv("ARGUS_DATABASE_METRIC_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("requires fresh native MongoDB/Elasticsearch/SQL Server/Oracle capture")
	}
	for _, key := range []string{"mongodb", "elasticsearch", "sqlserver", "oracle"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", filepath.Join(dir, key+".json"))
			testCapturedNativeTemplates(t, 1, 6)
		})
	}
}

func TestClickHouseNetworkLossDoesNotRenderZeroLatency(t *testing.T) {
	path := os.Getenv("ARGUS_NETWORK_PROBE_FAILURE_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh real ICMP packet-loss capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 3, 9, "icmp.overview/average", "icmp.overview/minimum", "icmp.overview/maximum", "icmp.overview/jitter")
}

func TestClickHouseOwnTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_BUILTIN_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native own metric capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 2, 12)
}

func TestClickHouseProcessServiceTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_PROCESS_SERVICE_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native process and real systemd capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 2, 8)
}

func TestClickHouseKubernetesTemplateCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_KUBERNETES_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh native Kubernetes metric capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 4, 24)
}

func TestClickHouseKubernetesControlTemplatesCapturedMetrics(t *testing.T) {
	path := os.Getenv("ARGUS_KUBERNETES_CONTROL_METRIC_EVIDENCE")
	if path == "" {
		t.Skip("requires fresh successful native control component capture")
	}
	t.Setenv("ARGUS_MONITORING_NATIVE_PAYLOADS", path)
	testCapturedNativeTemplates(t, 3, 18)
}

func testCapturedNativeTemplates(t *testing.T, wantSources, wantPanels int, emptyInstantPanels ...string) {
	path := os.Getenv("ARGUS_MONITORING_NATIVE_PAYLOADS")
	if path == "" {
		t.Skip("requires fresh native receiver capture")
	}
	conn := sourceTestClickHouse(t)
	var capture struct {
		CapturedAt  time.Time             `json:"captured_at"`
		CollectorID uuid.UUID             `json:"collector_id"`
		HostID      uuid.UUID             `json:"host_id"`
		ClusterID   uuid.UUID             `json:"cluster_id"`
		Sources     []configbundle.Source `json:"sources"`
		Requests    [][]byte              `json:"requests"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	if time.Since(capture.CapturedAt) > 45*time.Minute || time.Until(capture.CapturedAt) > time.Minute || len(capture.Requests) < 2 {
		t.Fatal("fresh multi-scrape native capture required")
	}
	var templates []struct {
		Key  string `json:"key"`
		Spec struct {
			Panels []struct {
				ID     string `json:"id"`
				Source struct {
					Type string `json:"source_type"`
				} `json:"source_binding"`
				Targets []struct {
					Mode       string `json:"query_mode"`
					Definition struct {
						DSL struct {
							Expression string `json:"expression"`
						} `json:"dsl"`
					} `json:"source_definition"`
				} `json:"targets"`
			} `json:"panels"`
		} `json:"spec"`
	}
	raw, err = os.ReadFile(filepath.Join("..", "..", "api", "contracts", "dashboard-templates.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &templates); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	enterprise := uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err = manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	keepForView := false
	t.Cleanup(func() {
		if keepForView {
			return
		}
		if err := manager.DropTenant(context.Background(), enterprise); err != nil {
			t.Error(err)
		}
	})
	control := nativeTemplateSourceControl{sourceTestControl{collector: capture.CollectorID, sources: map[uuid.UUID]SourceIdentity{}}}
	for _, item := range capture.Sources {
		id := uuid.MustParse(item.ID)
		control.sources[id] = SourceIdentity{ID: id, Type: item.Type, Revision: item.Revision, Generation: uuid.MustParse(item.Generation), ResourceID: uuid.MustParse(item.ResourceID), ResourceType: item.ResourceType, CollectionID: uuid.MustParse(item.CollectionID), CollectionRevision: item.CollectionRevision, AssignmentEpoch: item.AssignmentEpoch, CapabilityVersion: item.CapabilityVersion}
	}
	ingest := IngestServer{Control: control}
	identity := TrustedIdentity{EnterpriseID: enterprise, CollectorID: capture.CollectorID, ResourceID: capture.HostID, ResourceType: "host", Role: "direct"}
	if capture.ClusterID != uuid.Nil {
		identity.ResourceID, identity.ResourceType = capture.ClusterID, "kubernetes_cluster"
	}
	writer := Writer{ClickHouse: conn, Router: router}
	seen := map[uuid.UUID]bool{}
	apiCollections := map[uuid.UUID]bool{}
	for _, source := range control.sources {
		if source.Type == "kubernetes_apiserver" || source.Type == "kubernetes_scheduler" || source.Type == "kubernetes_controller_manager" || source.Type == "kubernetes_etcd" {
			apiCollections[source.CollectionID] = true
		}
	}
	apiFailed, apiRecovered := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	for i, payload := range capture.Requests {
		request := &metricsrpc.ExportMetricsServiceRequest{}
		if err = proto.Unmarshal(payload, request); err != nil {
			t.Fatal(err)
		}
		if _, err = ingest.prepareMetricPayload(ctx, identity, request); err != nil {
			t.Fatalf("capture identity: %v", err)
		}
		normalized, marshalErr := proto.Marshal(request)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, observation := range acceptedCollectionSamples("metrics", normalized) {
			if !apiCollections[observation.ID] {
				continue
			}
			if observation.Health == "failed" {
				if observation.HasData || observation.DataAt != nil {
					t.Fatal("actual denied API scrape fabricated target data")
				}
				apiFailed[observation.ID] = true
			}
			if apiFailed[observation.ID] && observation.Health == "healthy" && observation.HasData {
				apiRecovered[observation.ID] = true
			}
		}
		for _, resource := range request.ResourceMetrics {
			attrs := attributesMap(resource.Resource.Attributes)
			seen[uuid.MustParse(attrs["argus.source.id"])] = true
		}
		if _, err = writer.writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics", Offset: int64(i + 1), Timestamp: capture.CapturedAt, Headers: []kgo.RecordHeader{{Key: "argus-normalization-version", Value: []byte("v3")}}}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	for id := range apiCollections {
		if !apiFailed[id] || !apiRecovered[id] {
			t.Fatalf("actual API scrape %s omitted denial or recovery: denied=%v recovered=%v", id, apiFailed[id], apiRecovered[id])
		}
	}
	start, end := capture.CapturedAt.Add(-time.Minute), capture.CapturedAt.Add(time.Second)
	engine := promqlengine.NewEngine(conn, router, nil)
	panels := 0
	for _, template := range templates {
		for _, panel := range template.Spec.Panels {
			for id := range seen {
				source := control.sources[id]
				if source.Type != panel.Source.Type {
					continue
				}
				for _, target := range panel.Targets {
					// Match the bounded production Dashboard budget. API-server
					// histograms contain more dimensions than database fixtures.
					request := promqlengine.Request{Expression: target.Definition.DSL.Expression, Instant: target.Mode == "instant", Start: start, End: end, Step: time.Second, MaxSeries: DefaultMaxSeries, MaxSamples: DefaultMaxSamples, MaxScanBytes: DefaultMaxScanBytes, Timeout: DefaultTimeout, Scope: promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{source.ResourceID}, SourceKeys: []string{source.Key()}}}
					expectEmpty := slices.Contains(emptyInstantPanels, template.Key+"/"+panel.ID)
					if expectEmpty {
						request.Instant = true
					}
					result, err := engine.Execute(ctx, request)
					if err != nil {
						t.Fatalf("%s/%s: %v", template.Key, panel.ID, err)
					}
					points := 0
					switch values := result.Value.(type) {
					case promql.Vector:
						for _, point := range values {
							if math.IsNaN(point.F) || math.IsInf(point.F, 0) {
								t.Fatal("non-finite native statistic")
							}
							points++
						}
					case promql.Matrix:
						for _, series := range values {
							for _, point := range series.Floats {
								if math.IsNaN(point.F) || math.IsInf(point.F, 0) {
									t.Fatalf("%s/%s: non-finite native series", template.Key, panel.ID)
								}
								points++
							}
						}
					default:
						t.Fatalf("unexpected result type %T", result.Value)
					}
					if expectEmpty && points != 0 {
						t.Fatalf("%s/%s displayed RTT for a destination with complete packet loss", template.Key, panel.ID)
					}
					if !expectEmpty && points == 0 {
						t.Fatalf("%s/%s received no native samples", template.Key, panel.ID)
					}
					t.Logf("%s/%s target=%s points=%d", template.Key, panel.ID, source.ResourceID, points)
					panels++
					request.Scope.SourceKeys = []string{uuid.NewString() + ":1"}
					isolated, err := engine.Execute(ctx, request)
					if err != nil {
						t.Fatal(err)
					}
					if values, ok := isolated.Value.(promql.Vector); ok && len(values) != 0 {
						t.Fatal("unregistered source returned native metrics")
					}
					if values, ok := isolated.Value.(promql.Matrix); ok && len(values) != 0 {
						t.Fatal("unregistered source returned native series")
					}
				}
			}
		}
	}
	if len(seen) != wantSources || panels != wantPanels {
		t.Fatalf("native template coverage: sources=%d panels=%d", len(seen), panels)
	}
	if destination := os.Getenv("ARGUS_MONITORING_NATIVE_VIEW_RECEIPT"); destination != "" {
		receipt, err := json.MarshalIndent(map[string]any{"owner": "plan8-native-template-view/" + enterprise.String(), "enterprise_id": enterprise, "captured_at": capture.CapturedAt, "collector_id": capture.CollectorID, "host_id": capture.HostID, "sources": capture.Sources}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(destination, receipt, 0600); err != nil {
			t.Fatal(err)
		}
		keepForView = true
	}
}
