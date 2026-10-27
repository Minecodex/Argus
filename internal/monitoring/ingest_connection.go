package monitoring

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetrybinding"
	"github.com/kakj-go/Argus/internal/trustbundle"
)

// Public trust material only. Key reveal is a separate interactive operation.
type IngestTrustProvider interface {
	Current(context.Context) (trustbundle.Bundle, error)
}
type IngestListener struct {
	Transport     string `json:"transport"`
	Endpoint      string `json:"endpoint"`
	LocalEndpoint string `json:"local_endpoint"`
	Port          int32  `json:"port"`
	Path          string `json:"path"`
}
type IngestTLS struct {
	Required         bool     `json:"required"`
	MinimumVersion   string   `json:"minimum_version"`
	ServerName       string   `json:"server_name"`
	TrustBundleEpoch int64    `json:"trust_bundle_epoch"`
	CABundlePEM      string   `json:"ca_bundle_pem"`
	CABundleSHA256   string   `json:"ca_bundle_sha256"`
	CAFingerprints   []string `json:"ca_fingerprints"`
}
type IngestConnection struct {
	Name                 string           `json:"name"`
	EndpointID           uuid.UUID        `json:"endpoint_id"`
	ApplicationID        uuid.UUID        `json:"application_id"`
	CollectorID          uuid.UUID        `json:"collector_id"`
	Environment          string           `json:"environment"`
	Protocol             string           `json:"protocol"`
	Signals              []string         `json:"signals"`
	DesiredEnabled       bool             `json:"desired_enabled"`
	ConfigurationStatus  string           `json:"configuration_status"`
	CollectorStatus      string           `json:"collector_status"`
	ApplicationStatus    string           `json:"application_status"`
	ObservationState     string           `json:"observation_state"`
	LastReceivedAt       *time.Time       `json:"last_received_at,omitempty"`
	LastRejectedAt       *time.Time       `json:"last_rejected_at,omitempty"`
	LastErrorCode        string           `json:"last_error_code"`
	AuthenticationHeader string           `json:"authentication_header"`
	AuthenticationScheme string           `json:"authentication_scheme"`
	ServiceNameRequired  bool             `json:"service_name_required"`
	NetworkPrerequisite  string           `json:"network_prerequisite"`
	Listeners            []IngestListener `json:"listeners"`
	TLS                  IngestTLS        `json:"tls"`
}

func (s Service) IngestConnection(ctx context.Context, a Actor, id uuid.UUID) (IngestConnection, error) {
	out := IngestConnection{}
	if s.Store == nil || s.TrustBundles == nil {
		return out, ErrUnavailable
	}
	if id == uuid.Nil {
		return out, ErrInvalid
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "application.read"); err != nil {
		return out, err
	}
	endpoint, err := q.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return out, lookupError(err)
	}
	app, err := s.Application(ctx, a, endpoint.ApplicationID)
	if err != nil {
		return out, err
	}
	if !SDKProtocolEligible(endpoint.Protocol) {
		return out, ErrUnsupported
	}
	if !endpoint.CollectorID.Valid {
		return out, ErrUnavailable
	}
	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: endpoint.CollectorID.UUID})
	if err != nil {
		return out, lookupError(err)
	}
	distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
	if err != nil {
		return out, err
	}
	if !sdkDistributionEligible(endpoint.Protocol, distribution.Version, distribution.CollectorVersion, distribution.ConfigSchemaVersion, distribution.SupportStatus, distribution.Components) {
		return out, ErrUnsupported
	}
	bundle, err := s.TrustBundles.Current(ctx)
	if err != nil || bundle.Epoch < 1 || len(bundle.Material.PEM) == 0 {
		return out, ErrUnavailable
	}
	out = IngestConnection{Name: endpoint.Name, EndpointID: id, ApplicationID: app.ID, CollectorID: collector.ID, Environment: app.Environment, Protocol: endpoint.Protocol, Signals: append([]string{}, endpoint.Signals...), DesiredEnabled: endpoint.Enabled, ConfigurationStatus: "pending", CollectorStatus: collector.Status, ApplicationStatus: app.Status, ObservationState: "waiting", LastReceivedAt: ingestTime(endpoint.LastReceivedAt), LastRejectedAt: ingestTime(endpoint.LastRejectedAt), LastErrorCode: endpoint.LastErrorCode, AuthenticationHeader: "authorization", AuthenticationScheme: "Bearer", ServiceNameRequired: true, NetworkPrerequisite: "resolve_collector_dns_or_use_local_address_with_tls_server_name", Listeners: []IngestListener{}, TLS: IngestTLS{Required: true, MinimumVersion: "1.3", ServerName: telemetrybinding.CollectorServerName(collector.ID), TrustBundleEpoch: bundle.Epoch, CABundlePEM: string(bundle.Material.PEM), CABundleSHA256: bundle.Material.SHA256, CAFingerprints: append([]string{}, bundle.Material.Fingerprints...)}}
	if out.LastReceivedAt != nil {
		out.ObservationState = "received"
	}
	if out.LastRejectedAt != nil && (out.LastReceivedAt == nil || out.LastRejectedAt.After(*out.LastReceivedAt)) {
		out.ObservationState = "rejected"
	}
	if collector.Status == "uninstalled" || collector.Status == "uninstalling" {
		out.ConfigurationStatus = "unavailable"
	} else if collector.EffectiveRevision > 0 {
		config, e := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector.ID, Revision: collector.EffectiveRevision})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return IngestConnection{}, e
		}
		if e == nil {
			sources, e := configbundle.Sources(config.RenderedConfig)
			if e != nil {
				out.ConfigurationStatus = "unknown"
			} else {
				found, versionMatches := false, false
				for _, source := range sources {
					if source.IngestEndpointID == id.String() {
						found = true
						versionMatches = source.IngestEndpointVersion == endpoint.Version
					}
				}
				if endpoint.Enabled && versionMatches || !endpoint.Enabled && !found {
					out.ConfigurationStatus = "effective"
				}
			}
		}
	}
	// Canonical certificate DNS is the shared address contract. Never derive
	// or disclose an infrastructure IP through an application-only data grant.
	// The operator configures DNS/routing; local SDKs can use the loopback form
	// only while retaining the explicit TLS server name and public trust bundle.
	for _, port := range configbundle.ApplicationProtocolPorts(endpoint.Protocol) {
		remote := net.JoinHostPort(out.TLS.ServerName, strconv.Itoa(int(port.Port)))
		local := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port.Port)))
		scheme := "https://"
		if port.Transport == "grpc" {
			scheme = "grpcs://"
		}
		out.Listeners = append(out.Listeners, IngestListener{Transport: port.Transport, Endpoint: scheme + remote + port.Path, LocalEndpoint: scheme + local + port.Path, Port: port.Port, Path: port.Path})
	}
	if _, err = s.Application(ctx, a, endpoint.ApplicationID); err != nil {
		return IngestConnection{}, err
	}
	return out, nil
}
