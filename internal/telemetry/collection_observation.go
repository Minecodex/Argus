package telemetry

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

type CollectionSample struct {
	ID         uuid.UUID  `json:"collection_id"`
	Revision   int64      `json:"revision"`
	Epoch      int64      `json:"epoch"`
	Health     string     `json:"health"`
	ErrorCode  string     `json:"error_code"`
	HasData    bool       `json:"has_data"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	DataAt     *time.Time `json:"data_at,omitempty"`
	hasUp      bool
}
type collectionSampleObserver interface {
	ObserveCollectionSamples(context.Context, TrustedIdentity, []CollectionSample) error
}

func (control PostgresIngestControl) ObserveCollectionSamples(ctx context.Context, identity TrustedIdentity, samples []CollectionSample) error {
	if len(samples) == 0 {
		return nil
	}
	raw, err := json.Marshal(samples)
	if err != nil {
		return err
	}
	_, err = control.Queries.ObserveMonitoringSamples(ctx, db.ObserveMonitoringSamplesParams{EnterpriseID: identity.EnterpriseID, CollectorID: uuid.NullUUID{UUID: identity.CollectorID, Valid: true}, Samples: raw})
	return err
}

func acceptedCollectionSamples(signal string, payload []byte) []CollectionSample {
	var resources []*resourcepb.Resource
	diagnostics := map[*resourcepb.Resource]collectionMetricDiagnostic{}
	if signal == "metrics" {
		value := &metrics.ExportMetricsServiceRequest{}
		if proto.Unmarshal(payload, value) != nil {
			return nil
		}
		for _, item := range value.ResourceMetrics {
			if item == nil || item.Resource == nil {
				continue
			}
			diagnostic := nativeScrapeDiagnostic(item)
			if !diagnostic.hasPoint {
				continue
			}
			resources = append(resources, item.Resource)
			diagnostics[item.Resource] = diagnostic
		}
	} else {
		resources = acceptedTelemetryResources(signal, payload)
	}
	type identityKey struct {
		ID              uuid.UUID
		Revision, Epoch int64
	}
	seen := map[identityKey]int{}
	result := []CollectionSample{}
	for _, resource := range resources {
		attrs := attributesMap(resource.GetAttributes())
		id, err := uuid.Parse(attrs["argus.collection.id"])
		if err != nil || id == uuid.Nil {
			continue
		}
		revision, e1 := strconv.ParseInt(attrs["argus.collection.revision"], 10, 64)
		epoch, e2 := strconv.ParseInt(attrs["argus.assignment.epoch"], 10, 64)
		if e1 != nil || e2 != nil || revision < 1 || epoch < 1 {
			continue
		}
		item := CollectionSample{ID: id, Revision: revision, Epoch: epoch, Health: "healthy", HasData: true}
		if diagnostic, ok := diagnostics[resource]; ok {
			item.Health, item.ErrorCode, item.HasData, item.ObservedAt, item.DataAt = diagnostic.health, diagnostic.code, diagnostic.hasData, diagnostic.observedAt, diagnostic.dataAt
			item.hasUp = diagnostic.hasUp
		}
		key := identityKey{id, revision, epoch}
		if index, ok := seen[key]; !ok {
			seen[key] = len(result)
			result = append(result, item)
		} else {
			current := &result[index]
			current.HasData = current.HasData || item.HasData
			current.hasUp = current.hasUp || item.hasUp
			if item.ObservedAt != nil && (current.ObservedAt == nil || item.ObservedAt.After(*current.ObservedAt)) {
				current.ObservedAt = item.ObservedAt
			}
			if item.DataAt != nil && (current.DataAt == nil || item.DataAt.After(*current.DataAt)) {
				current.DataAt = item.DataAt
			}
			if item.Health == "failed" || current.Health == "failed" || item.Health == "degraded" || current.Health == "degraded" {
				current.Health, current.ErrorCode = "failed", "COLLECTION_SCRAPE_FAILED"
				if current.HasData || current.hasUp {
					current.Health, current.ErrorCode = "degraded", "COLLECTION_SCRAPE_PARTIAL_FAILURE"
				}
			}
		}
	}
	return result
}

func acceptedTelemetryResources(signal string, payload []byte) []*resourcepb.Resource {
	resources := []*resourcepb.Resource{}
	switch signal {
	case "metrics":
		value := &metrics.ExportMetricsServiceRequest{}
		if proto.Unmarshal(payload, value) != nil {
			return nil
		}
		for _, item := range value.ResourceMetrics {
			count := 0
			for _, scope := range item.ScopeMetrics {
				for _, metric := range scope.Metrics {
					count += len(metric.GetGauge().GetDataPoints()) + len(metric.GetSum().GetDataPoints()) + len(metric.GetHistogram().GetDataPoints()) + len(metric.GetExponentialHistogram().GetDataPoints()) + len(metric.GetSummary().GetDataPoints())
				}
			}
			if count > 0 {
				resources = append(resources, item.Resource)
			}
		}
	case "logs":
		value := &logs.ExportLogsServiceRequest{}
		if proto.Unmarshal(payload, value) != nil {
			return nil
		}
		for _, item := range value.ResourceLogs {
			count := 0
			for _, scope := range item.ScopeLogs {
				count += len(scope.LogRecords)
			}
			if count > 0 {
				resources = append(resources, item.Resource)
			}
		}
	case "traces":
		value := &traces.ExportTraceServiceRequest{}
		if proto.Unmarshal(payload, value) != nil {
			return nil
		}
		for _, item := range value.ResourceSpans {
			count := 0
			for _, scope := range item.ScopeSpans {
				count += len(scope.Spans)
			}
			if count > 0 {
				resources = append(resources, item.Resource)
			}
		}
	}
	return resources
}
