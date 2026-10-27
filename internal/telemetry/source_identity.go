package telemetry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

type SourceIdentity struct {
	ServiceID                                                uuid.UUID
	ServiceName, ServiceNamespace, Environment, NamespaceUID string
	NamespaceUnverified                                      bool
	ID                                                       uuid.UUID
	Revision                                                 int64
	Type                                                     string
	Generation                                               uuid.UUID
	CapabilityVersion                                        string
	ResourceID                                               uuid.UUID
	ResourceType                                             string
	CollectionID                                             uuid.UUID
	CollectionRevision                                       int64
	AssignmentEpoch                                          int64
	IngestEndpointID                                         uuid.UUID
}

func (source SourceIdentity) Key() string {
	return source.ID.String() + ":" + strconv.FormatInt(source.Revision, 10)
}

type IngestSourceResolver interface {
	ResolveSource(context.Context, TrustedIdentity, uuid.UUID, int64, string) (SourceIdentity, error)
}

func (store PostgresIngestControl) ResolveSource(ctx context.Context, identity TrustedIdentity, id uuid.UUID, revision int64, signal string) (SourceIdentity, error) {
	row, err := store.Queries.ResolveTelemetrySource(ctx, db.ResolveTelemetrySourceParams{ID: id, ConfigRevision: revision, CollectorID: identity.CollectorID, EnterpriseID: identity.EnterpriseID})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return SourceIdentity{}, controlReadError(err)
		}
		pending, lookupErr := store.Queries.TelemetrySourceAwaitingApply(ctx, db.TelemetrySourceAwaitingApplyParams{ID: id, ConfigRevision: revision, CollectorID: identity.CollectorID, EnterpriseID: identity.EnterpriseID, Signal: signal})
		if lookupErr != nil {
			return SourceIdentity{}, controlReadError(lookupErr)
		}
		if pending {
			return SourceIdentity{}, ErrSourceAwaitingApply
		}
		return SourceIdentity{}, errIngestRejected
	}
	if !slices.Contains(row.Signals, signal) {
		return SourceIdentity{}, errIngestRejected
	}
	result := SourceIdentity{ID: row.ID, Revision: row.ConfigRevision, Type: row.SourceType, Generation: row.Generation, CapabilityVersion: row.CapabilityVersion,
		ResourceID: row.ResourceID, ResourceType: row.ResourceType, CollectionID: row.CollectionID.UUID, CollectionRevision: row.CollectionRevision.Int64, AssignmentEpoch: row.AssignmentEpoch.Int64, IngestEndpointID: row.IngestEndpointID.UUID, ServiceID: row.ApplicationServiceID.UUID, NamespaceUID: row.NamespaceUid}
	if row.ApplicationServiceID.Valid {
		if signal != "logs" || row.ResourceType != "application" || !row.CollectionID.Valid || row.IngestEndpointID.Valid {
			return SourceIdentity{}, errIngestRejected
		}
		service, e := store.Queries.GetMonitoringService(ctx, db.GetMonitoringServiceParams{EnterpriseID: identity.EnterpriseID, ID: row.ApplicationServiceID.UUID})
		if e != nil {
			return SourceIdentity{}, controlReadError(e)
		}
		if service.Status != "active" || service.ApplicationID != row.ResourceID {
			return SourceIdentity{}, errIngestRejected
		}
		app, e := store.Queries.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: identity.EnterpriseID, ID: row.ResourceID})
		if e != nil {
			return SourceIdentity{}, controlReadError(e)
		}
		if app.Status != "active" {
			return SourceIdentity{}, errIngestRejected
		}
		result.ServiceName, result.ServiceNamespace, result.Environment = service.ServiceName, service.ServiceNamespace, app.Environment
	}
	return result, nil
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
		if err != nil || !present {
			return errIngestRejected
		}
		sources[i] = SourceIdentity{Type: "unknown"}
		if present {
			resolver, ok := server.Control.(IngestSourceResolver)
			if !ok {
				return errIngestRejected
			}
			sources[i], err = resolver.ResolveSource(ctx, identity, id, revision, signal)
			if err != nil {
				return &admissionStageError{Stage: "source", Cause: err}
			}
			if sources[i].ID != id || sources[i].Revision != revision || sources[i].Type == "" {
				return errIngestRejected
			}
			if signal == "metrics" {
				sources[i].NamespaceUID, err = server.metricNamespace(ctx, identity, sources[i], (*res).GetAttributes())
				if errors.Is(err, errNamespaceAttributionUnverified) {
					sources[i].NamespaceUID = ""
					sources[i].NamespaceUnverified = true
					err = nil
				}
				if err != nil {
					stage := "namespace_uid"
					if attributesMap((*res).GetAttributes())["k8s.namespace.uid"] == "" {
						stage = "namespace_object"
					}
					return &admissionStageError{Stage: stage, Cause: err}
				}
			}
			source := sources[i]
			if source.ServiceID != uuid.Nil && (signal != "logs" || source.ResourceType != "application" || source.IngestEndpointID != uuid.Nil || source.CollectionID == uuid.Nil || source.ServiceName == "" || source.Environment == "") {
				return errIngestRejected
			}
			if source.ResourceType == "application" && source.IngestEndpointID == uuid.Nil && source.ServiceID == uuid.Nil {
				return errIngestRejected
			}
		}
		// A Collector certificate authenticates the transport, not an SDK's
		// application. All business traces require a registered application
		// endpoint whose Key is rechecked below. Legacy host receivers cannot
		// turn arbitrary service.name labels into application authority.
		if signal == "traces" && (sources[i].IngestEndpointID == uuid.Nil || sources[i].ResourceType != "application" || sources[i].Type == "skywalking") {
			return errIngestRejected
		}
		if (sources[i].Type == "otlp" || sources[i].Type == "jaeger" || sources[i].Type == "skywalking") && sources[i].IngestEndpointID == uuid.Nil {
			return errIngestRejected
		}
	}
	applications, err := server.resolveApplicationSources(ctx, identity, signal, sources, resources)
	if err != nil {
		return &admissionStageError{Stage: "application_identity", Cause: err}
	}
	for i, res := range resources {
		source := sources[i]
		resourceIdentity := identity
		if source.ResourceID != uuid.Nil {
			resourceIdentity.ResourceID, resourceIdentity.ResourceType = source.ResourceID, source.ResourceType
		}
		overwriteResource(res, resourceIdentity)
		if application, ok := applications[i]; ok {
			stampApplicationIdentity(*res, application)
		}
		if source.ServiceID != uuid.Nil {
			stampApplicationIdentity(*res, monitoring.ApplicationIdentity{ApplicationID: source.ResourceID, ServiceID: source.ServiceID, Environment: source.Environment})
			setResourceString(*res, "service.name", source.ServiceName)
			setResourceString(*res, "service.namespace", source.ServiceNamespace)
		}
		if source.NamespaceUID != "" {
			setResourceString(*res, "argus.namespace.uid", source.NamespaceUID)
		}
		if source.NamespaceUnverified {
			setResourceString(*res, "argus.namespace.attribution_status", "unverified")
		}
		(*res).Attributes = append((*res).Attributes,
			&commonpb.KeyValue{Key: "argus.source.id", Value: stringValue(source.ID.String())},
			&commonpb.KeyValue{Key: "argus.source.revision", Value: stringValue(strconv.FormatInt(source.Revision, 10))},
			&commonpb.KeyValue{Key: "argus.source.type", Value: stringValue(source.Type)},
			&commonpb.KeyValue{Key: "argus.source.generation", Value: stringValue(source.Generation.String())},
			&commonpb.KeyValue{Key: "argus.source.capability_version", Value: stringValue(source.CapabilityVersion)},
		)
		if source.CollectionID != uuid.Nil {
			(*res).Attributes = append((*res).Attributes,
				&commonpb.KeyValue{Key: "argus.collection.id", Value: stringValue(source.CollectionID.String())},
				&commonpb.KeyValue{Key: "argus.collection.revision", Value: stringValue(strconv.FormatInt(source.CollectionRevision, 10))},
				&commonpb.KeyValue{Key: "argus.assignment.epoch", Value: stringValue(strconv.FormatInt(source.AssignmentEpoch, 10))})
		}
	}
	return nil
}

func (server *IngestServer) prepareMetricPayload(ctx context.Context, gateway TrustedIdentity, request *collectmetrics.ExportMetricsServiceRequest) (TrustedIdentity, error) {
	identity, err := server.resolveMetricPayloadIdentity(ctx, gateway, request)
	if err != nil {
		return identity, &admissionStageError{Stage: "forwarded_identity", Cause: err}
	}
	resources := []**resourcepb.Resource{}
	for _, item := range request.ResourceMetrics {
		if item == nil {
			return identity, errIngestRejected
		}
		resources = append(resources, &item.Resource)
	}
	if err := server.applyTrustedSources(ctx, identity, "metrics", resources); err != nil {
		return identity, err
	}
	return identity, redactMetricPayload(request)
}
func (server *IngestServer) prepareLogPayload(ctx context.Context, gateway TrustedIdentity, request *collectlogs.ExportLogsServiceRequest) (TrustedIdentity, error) {
	identity, err := server.resolveLogPayloadIdentity(ctx, gateway, request)
	if err != nil {
		return identity, &admissionStageError{Stage: "forwarded_identity", Cause: err}
	}
	resources := []**resourcepb.Resource{}
	for _, item := range request.ResourceLogs {
		if item == nil {
			return identity, errIngestRejected
		}
		resources = append(resources, &item.Resource)
	}
	if err := server.applyTrustedSources(ctx, identity, "logs", resources); err != nil {
		return identity, err
	}
	return identity, redactLogPayload(request)
}
func (server *IngestServer) prepareTracePayload(ctx context.Context, gateway TrustedIdentity, request *collecttraces.ExportTraceServiceRequest) (TrustedIdentity, error) {
	identity, err := server.resolveTracePayloadIdentity(ctx, gateway, request)
	if err != nil {
		return identity, &admissionStageError{Stage: "forwarded_identity", Cause: err}
	}
	resources := []**resourcepb.Resource{}
	for _, item := range request.ResourceSpans {
		if item == nil {
			return identity, errIngestRejected
		}
		resources = append(resources, &item.Resource)
	}
	if err := server.applyTrustedSources(ctx, identity, "traces", resources); err != nil {
		return identity, err
	}
	return identity, redactTracePayload(request)
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
