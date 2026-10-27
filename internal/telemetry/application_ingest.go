package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func (server *IngestServer) httpAuthorizeApplication(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.TLS == nil || len(r.TLS.PeerCertificates) != 1 || server.Applications.Store == nil || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
		http.Error(w, "application ingestion authentication rejected", http.StatusUnauthorized)
		return
	}
	identity, err := server.resolveIdentity(r.Context(), r.TLS.PeerCertificates[0])
	if err != nil {
		admissionHTTPError(w, err, "application ingestion authentication rejected", http.StatusUnauthorized)
		return
	}
	if server.Redis == nil {
		http.Error(w, "application ingestion authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	rateKey := "argus:application-ingest:auth:" + identity.CollectorID.String() + ":" + time.Now().UTC().Format("200601021504")
	count, err := server.Redis.Raw.Incr(r.Context(), rateKey).Result()
	if err != nil {
		http.Error(w, "application ingestion authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	if count == 1 {
		_ = server.Redis.Raw.Expire(r.Context(), rateKey, 2*time.Minute).Err()
	}
	if count > 2400 {
		http.Error(w, "application ingestion authentication rate exceeded", http.StatusTooManyRequests)
		return
	}
	var input struct {
		Protocol string `json:"protocol"`
		Key      string `json:"key"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "application ingestion authentication rejected", http.StatusBadRequest)
		return
	}
	key, err := monitoring.AuthenticateIngestKey(r.Context(), server.Applications.Store.Queries, identity.CollectorID, identity.EnterpriseID, input.Protocol, "", input.Key)
	if err != nil {
		if errors.Is(err, monitoring.ErrIngestRejected) {
			_ = server.Applications.ObserveRejectedIngestKey(r.Context(), identity.EnterpriseID, identity.CollectorID, input.Protocol, input.Key)
			input.Key = ""
			http.Error(w, "application ingestion authentication rejected", http.StatusUnauthorized)
		} else {
			input.Key = ""
			http.Error(w, "application ingestion authentication unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	input.Key = ""
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"key_id": key.ID.String(), "endpoint_id": key.EndpointID.String(), "endpoint_version": strconv.FormatInt(key.EndpointVersion, 10), "signals": strings.Join(key.Signals, ",")})
}

func (server *IngestServer) resolveApplicationSources(ctx context.Context, identity TrustedIdentity, signal string, sources []SourceIdentity, resources []**resourcepb.Resource) (map[int]monitoring.ApplicationIdentity, error) {
	positions := []int{}
	observations := []monitoring.IngestObservation{}
	for index, source := range sources {
		if source.IngestEndpointID == uuid.Nil {
			continue
		}
		attrs := attributesMap((*resources[index]).GetAttributes())
		key, err := uuid.Parse(attrs["argus.application.key.id"])
		if err != nil || key == uuid.Nil {
			return nil, errIngestRejected
		}
		if old, current := attrs["deployment.environment"], attrs["deployment.environment.name"]; old != "" && current != "" && old != current {
			return nil, errIngestRejected
		}
		environment := attrs["deployment.environment.name"]
		if environment == "" {
			environment = attrs["deployment.environment"]
		}
		positions = append(positions, index)
		observations = append(observations, monitoring.IngestObservation{KeyID: key, EndpointID: source.IngestEndpointID, ApplicationID: source.ResourceID, Protocol: source.Type, ServiceNamespace: attrs["service.namespace"], ServiceName: attrs["service.name"], InstanceKey: attrs["service.instance.id"], Runtime: attrs["telemetry.sdk.language"], Environment: environment})
	}
	result := map[int]monitoring.ApplicationIdentity{}
	if len(observations) == 0 {
		return result, nil
	}
	resolved, err := server.Applications.RegisterIngestResources(ctx, identity.EnterpriseID, identity.CollectorID, signal, observations)
	if err != nil {
		if errors.Is(err, monitoring.ErrIngestRejected) {
			return nil, errIngestRejected
		}
		return nil, ErrUnavailable
	}
	for index, value := range resolved {
		result[positions[index]] = value
	}
	return result, nil
}

func stampApplicationIdentity(resource *resourcepb.Resource, identity monitoring.ApplicationIdentity) {
	values := map[string]string{"argus.application.id": identity.ApplicationID.String(), "argus.service.id": identity.ServiceID.String(), "deployment.environment.name": identity.Environment, "deployment.environment": identity.Environment}
	if identity.EndpointID != uuid.Nil {
		values["argus.application.endpoint.id"] = identity.EndpointID.String()
	}
	if identity.KeyID != uuid.Nil {
		values["argus.application.key.id"] = identity.KeyID.String()
	}
	for key, value := range values {
		setResourceString(resource, key, value)
	}
	if identity.InstanceID != uuid.Nil {
		resource.Attributes = append(resource.Attributes, &commonpb.KeyValue{Key: "argus.service.instance.id", Value: stringValue(identity.InstanceID.String())})
	}
}

func setResourceString(resource *resourcepb.Resource, key, value string) {
	filtered := resource.Attributes[:0]
	for _, attr := range resource.Attributes {
		if attr != nil && attr.Key != key {
			filtered = append(filtered, attr)
		}
	}
	resource.Attributes = append(filtered, &commonpb.KeyValue{Key: key, Value: stringValue(value)})
}
