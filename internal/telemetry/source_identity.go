package telemetry

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

type SourceIdentity struct {
	ID                uuid.UUID
	Revision          int64
	Type              string
	Generation        uuid.UUID
	CapabilityVersion string
}

func (source SourceIdentity) Key() string {
	return source.ID.String() + ":" + strconv.FormatInt(source.Revision, 10)
}

type IngestSourceResolver interface {
	ResolveSource(context.Context, TrustedIdentity, uuid.UUID, int64, string) (SourceIdentity, error)
}

func (store PostgresIngestControl) ResolveSource(ctx context.Context, identity TrustedIdentity, id uuid.UUID, revision int64, signal string) (SourceIdentity, error) {
	row, err := store.Queries.ResolveTelemetrySource(ctx, db.ResolveTelemetrySourceParams{ID: id, ConfigRevision: revision, CollectorID: identity.CollectorID, EnterpriseID: identity.EnterpriseID})
	if err != nil || row.ResourceID != identity.ResourceID || row.ResourceType != identity.ResourceType || !slices.Contains(row.Signals, signal) {
		return SourceIdentity{}, errIngestRejected
	}
	return SourceIdentity{ID: row.ID, Revision: row.ConfigRevision, Type: row.SourceType, Generation: row.Generation, CapabilityVersion: row.CapabilityVersion}, nil
}

func sourceReference(attrs []*commonpb.KeyValue) (uuid.UUID, int64, bool, error) {
	idText, revisionText := "", ""
	seen := map[string]bool{}
	for _, attr := range attrs {
		if attr == nil {
			return uuid.Nil, 0, false, errIngestRejected
		}
		if attr.Key != "argus.source.id" && attr.Key != "argus.source.revision" {
			continue
		}
		if seen[attr.Key] {
			return uuid.Nil, 0, false, errIngestRejected
		}
		seen[attr.Key] = true
		value, ok := attr.Value.GetValue().(*commonpb.AnyValue_StringValue)
		if !ok {
			return uuid.Nil, 0, false, errIngestRejected
		}
		if attr.Key == "argus.source.id" {
			idText = value.StringValue
		} else {
			revisionText = value.StringValue
		}
	}
	if len(seen) == 0 {
		return uuid.Nil, 0, false, nil
	}
	id, err := uuid.Parse(idText)
	revision, e := strconv.ParseInt(revisionText, 10, 64)
	if err != nil || e != nil || id == uuid.Nil || revision < 1 {
		return uuid.Nil, 0, false, errIngestRejected
	}
	return id, revision, true, nil
}

func (server *IngestServer) applyTrustedSources(ctx context.Context, identity TrustedIdentity, signal string, resources []**resourcepb.Resource) error {
	sources := make([]SourceIdentity, len(resources))
	for i, res := range resources {
		id, revision, present, err := sourceReference((*res).GetAttributes())
		if err != nil {
			return err
		}
		sources[i] = SourceIdentity{Type: "unknown"}
		if present {
			resolver, ok := server.Control.(IngestSourceResolver)
			if !ok {
				return errIngestRejected
			}
			sources[i], err = resolver.ResolveSource(ctx, identity, id, revision, signal)
			if err != nil {
				return err
			}
			if sources[i].ID != id || sources[i].Revision != revision || sources[i].Type == "" {
				return errIngestRejected
			}
		}
	}
	for i, res := range resources {
		overwriteResource(res, identity)
		source := sources[i]
		(*res).Attributes = append((*res).Attributes,
			&commonpb.KeyValue{Key: "argus.source.id", Value: stringValue(source.ID.String())},
			&commonpb.KeyValue{Key: "argus.source.revision", Value: stringValue(strconv.FormatInt(source.Revision, 10))},
			&commonpb.KeyValue{Key: "argus.source.type", Value: stringValue(source.Type)},
			&commonpb.KeyValue{Key: "argus.source.generation", Value: stringValue(source.Generation.String())},
			&commonpb.KeyValue{Key: "argus.source.capability_version", Value: stringValue(source.CapabilityVersion)},
		)
	}
	return nil
}

func (server *IngestServer) prepareMetricPayload(ctx context.Context, gateway TrustedIdentity, request *collectmetrics.ExportMetricsServiceRequest) (TrustedIdentity, error) {
	identity, err := server.resolveMetricPayloadIdentity(ctx, gateway, request)
	if err != nil {
		return identity, err
	}
	resources := []**resourcepb.Resource{}
	for _, item := range request.ResourceMetrics {
		if item == nil {
			return identity, errIngestRejected
		}
		resources = append(resources, &item.Resource)
	}
	return identity, server.applyTrustedSources(ctx, identity, "metrics", resources)
}
func (server *IngestServer) prepareLogPayload(ctx context.Context, gateway TrustedIdentity, request *collectlogs.ExportLogsServiceRequest) (TrustedIdentity, error) {
	identity, err := server.resolveLogPayloadIdentity(ctx, gateway, request)
	if err != nil {
		return identity, err
	}
	resources := []**resourcepb.Resource{}
	for _, item := range request.ResourceLogs {
		if item == nil {
			return identity, errIngestRejected
		}
		resources = append(resources, &item.Resource)
	}
	return identity, server.applyTrustedSources(ctx, identity, "logs", resources)
}
func (server *IngestServer) prepareTracePayload(ctx context.Context, gateway TrustedIdentity, request *collecttraces.ExportTraceServiceRequest) (TrustedIdentity, error) {
	identity, err := server.resolveTracePayloadIdentity(ctx, gateway, request)
	if err != nil {
		return identity, err
	}
	resources := []**resourcepb.Resource{}
	for _, item := range request.ResourceSpans {
		if item == nil {
			return identity, errIngestRejected
		}
		resources = append(resources, &item.Resource)
	}
	return identity, server.applyTrustedSources(ctx, identity, "traces", resources)
}

// Kafka payloads are already normalized by Ingest. Missing origin in legacy
// records stays explicitly unknown, never inferred from SDK-provided labels.
func recordSource(attrs []*commonpb.KeyValue) (SourceIdentity, error) {
	values := attributesMap(attrs)
	source := SourceIdentity{Type: "unknown"}
	if values["argus.source.id"] == "" {
		return source, nil
	}
	var err error
	source.ID, err = uuid.Parse(values["argus.source.id"])
	if err != nil {
		return source, fmt.Errorf("%w: source id", errPermanentRecord)
	}
	source.Revision, err = strconv.ParseInt(values["argus.source.revision"], 10, 64)
	if err != nil || source.Revision < 0 || (source.ID == uuid.Nil) != (source.Revision == 0) {
		return source, fmt.Errorf("%w: source revision", errPermanentRecord)
	}
	source.Type = values["argus.source.type"]
	source.CapabilityVersion = values["argus.source.capability_version"]
	if source.Type == "" {
		return source, fmt.Errorf("%w: source type", errPermanentRecord)
	}
	return source, nil
}
