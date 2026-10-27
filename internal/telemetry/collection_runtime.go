package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type CollectionRuntime struct {
	Actions         ActionExtension
	KubernetesImage string
}

type collectorPlanningError struct {
	stage string
	cause error
}

func (e collectorPlanningError) Error() string          { return "Collector planning unavailable at " + e.stage }
func (e collectorPlanningError) Unwrap() error          { return e.cause }
func (e collectorPlanningError) OperationStage() string { return "collector_" + e.stage }

func (runtime CollectionRuntime) Plan(ctx context.Context, q *db.Queries, enterprise, id uuid.UUID) (body json.RawMessage, err error) {
	stage := "instance"
	defer func() {
		if err != nil {
			var staged interface{ OperationStage() string }
			if !errors.As(err, &staged) {
				err = collectorPlanningError{stage: stage, cause: err}
			}
		}
	}()

	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: enterprise, ID: id})
	if err != nil {
		return nil, err
	}
	stage = "configuration"
	config, err := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: id, Revision: collector.DesiredRevision})
	if err != nil {
		return nil, err
	}
	stage = "distribution"
	distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
	if err != nil {
		return nil, err
	}
	stage = "route"
	route, err := q.GetTelemetryRouteByCollector(ctx, db.GetTelemetryRouteByCollectorParams{EnterpriseID: enterprise, CollectorID: id})
	if err != nil {
		return nil, err
	}
	stage = "execution_target"
	target, err := resolveCollectorExecutionTarget(ctx, q, enterprise, collector.ResourceType, collector.ResourceID)
	if err != nil {
		return nil, err
	}
	stage = "tunnel_target"
	tunnel, err := resolveCollectorTunnelTarget(ctx, q, enterprise, collector.ResourceType, collector.ResourceID, route.Transport, route.Kind, int(route.LoopbackPort.Int32), target)
	if err != nil {
		return nil, err
	}
	stage = "artifacts"
	hashes, err := artifactHashes(distribution.ArtifactManifest)
	if err != nil {
		return nil, err
	}
	plan := collectorActionPlan{PlannedCollectorID: uuid.NullUUID{UUID: id, Valid: true}, Operation: "configure", ResourceType: collector.ResourceType, ResourceID: collector.ResourceID, DistributionVersionID: distribution.ID, ProfileIDs: slices.Clone(config.ProfileIds), RouteKind: route.Kind, RouteTransport: route.Transport, LoopbackPort: route.LoopbackPort.Int32, GatewayCollectorID: route.GatewayCollectorID, ExpectedVersion: collector.Version, ArtifactHashes: hashes, Platform: collector.Platform, Role: collector.Role,
		Transport: target.Transport, ConnectorID: target.ConnectorID, ConnectionEpoch: target.ConnectionEpoch, TargetResourceVersion: target.ResourceVersion, TargetAddress: target.Address, TargetPort: int32(target.Port), TargetUsername: target.Username, PinnedHostKey: target.PinnedHostKey, CredentialID: target.CredentialID, CredentialVersion: target.CredentialVersion,
		TunnelInitiator: tunnel.Initiator, TunnelConnectorID: tunnel.ConnectorID, TunnelTargetAddress: tunnel.Address, TunnelTargetPort: int32(tunnel.Port), TunnelTargetUsername: tunnel.Username, TunnelPinnedHostKey: tunnel.PinnedHostKey, TunnelCredentialID: tunnel.CredentialID, TunnelCredentialVersion: tunnel.CredentialVersion, EnrollmentDialAddress: tunnel.EnrollmentDialAddress}
	stage = "profiles"
	profiles, err := q.ListCollectionProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range plan.ProfileIDs {
		index := slices.IndexFunc(profiles, func(p db.CollectionProfile) bool { return p.ID == id })
		if index < 0 {
			return nil, resource.ErrActionInvalidated
		}
		plan.ProfileKeys = append(plan.ProfileKeys, profiles[index].ProfileKey+"@"+profiles[index].Version)
	}
	stage = "gateway"
	if route.Kind == "bastion_gateway" {
		plan.GatewayEndpoint, plan.GatewayServerName, err = telemetryGatewayEndpoint(ctx, q, enterprise, route.GatewayCollectorID, route.Transport)
		if err != nil {
			return nil, err
		}
	}
	stage = "kubernetes_image"
	if collector.ResourceType == "kubernetes_cluster" {
		// Keep the previous deployment's image and pull-secret references. They
		// are public deployment settings, not registry credential material.
		payload, e := q.GetLatestCollectorManagementPayload(ctx, db.GetLatestCollectorManagementPayloadParams{EnterpriseID: enterprise, CollectorID: id.String()})
		if e != nil {
			return nil, e
		}
		var previous struct {
			KubernetesImage  string   `json:"kubernetes_image"`
			ImagePullSecrets []string `json:"image_pull_secrets"`
		}
		if json.Unmarshal(payload, &previous) != nil {
			return nil, resource.ErrActionInvalidated
		}
		plan.KubernetesImage, plan.ImagePullSecrets = previous.KubernetesImage, previous.ImagePullSecrets
		if plan.KubernetesImage == "" {
			plan.KubernetesImage = runtime.KubernetesImage
		}
	}
	stage = "validation"
	if err = validateCollectorPlan(ctx, q, enterprise, plan); err != nil {
		return nil, err
	}
	return json.Marshal(plan)
}

func (runtime CollectionRuntime) Commit(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	var plan collectorActionPlan
	if json.Unmarshal(raw, &plan) != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	if err := validateCollectorPlan(ctx, q, action.EnterpriseID, plan); err != nil {
		return resource.ActionCommitResult{}, err
	}
	// One collection migration may configure two Collectors. Their command
	// identities must not collide even though the user confirmed one action.
	action.ActionRef += "_" + plan.PlannedCollectorID.UUID.String()
	return runtime.Actions.commitCollectorAction(ctx, q, action, plan)
}

func collectionSnapshots(ctx context.Context, q *db.Queries, enterprise, collector uuid.UUID) ([]configbundle.Collection, error) {
	rows, err := q.ListCollectorMonitoringCollections(ctx, db.ListCollectorMonitoringCollectionsParams{EnterpriseID: enterprise, CollectorID: uuid.NullUUID{UUID: collector, Valid: true}})
	if err != nil {
		return nil, err
	}
	result := make([]configbundle.Collection, 0, len(rows))
	for _, row := range rows {
		if row.CollectorID.Valid && row.CollectorID.UUID != collector {
			// Preserve the exact last verified placement while a replacement is
			// starting. Never rebuild it from a newer target/configuration draft.
			instance, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: enterprise, ID: collector})
			if err != nil {
				return nil, err
			}
			config, err := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector, Revision: instance.EffectiveRevision})
			if err != nil {
				return nil, err
			}
			var bundle configbundle.Bundle
			if json.Unmarshal(config.RenderedConfig, &bundle) != nil {
				return nil, resource.ErrActionInvalidated
			}
			found := false
			for _, item := range bundle.Collections {
				if item.ID == row.ID.String() && item.Revision == row.EffectiveRevision && item.AssignmentEpoch == row.EffectiveAssignmentEpoch {
					result = append(result, item)
					found = true
					break
				}
			}
			if !found {
				return nil, resource.ErrActionInvalidated
			}
			continue
		}
		if _, ok := builtincollection.Lookup(row.IntegrationKey); ok {
			result = append(result, configbundle.Collection{ID: row.ID.String(), Integration: row.IntegrationKey, TargetID: row.TargetID.String(), TargetType: row.TargetType, Identity: json.RawMessage(`{}`), Config: row.Config, Revision: row.DesiredRevision, AssignmentEpoch: row.AssignmentEpoch, Enabled: row.DesiredEnabled})
			continue
		}
		if logcollection.IsIntegration(row.IntegrationKey) {
			var logRule logcollection.Config
			if json.Unmarshal(row.Config, &logRule) != nil {
				return nil, resource.ErrActionInvalidated
			}
			item := configbundle.Collection{ID: row.ID.String(), Integration: row.IntegrationKey, TargetID: row.TargetID.String(), TargetType: row.TargetType, Identity: json.RawMessage(`{}`), Config: row.Config, Revision: row.DesiredRevision, AssignmentEpoch: row.AssignmentEpoch, Enabled: row.DesiredEnabled}
			if logRule.Workload != nil {
				item.NamespaceUID = logRule.Workload.NamespaceUID
			}
			if row.TargetType == "application" {
				if !row.ServiceID.Valid || row.ApplicationID.UUID != row.TargetID {
					return nil, resource.ErrActionInvalidated
				}
				service, e := q.GetMonitoringService(ctx, db.GetMonitoringServiceParams{EnterpriseID: enterprise, ID: row.ServiceID.UUID})
				if e != nil || service.ApplicationID != row.TargetID || service.Status != "active" {
					return nil, resource.ErrActionInvalidated
				}
				app, e := q.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: enterprise, ID: row.TargetID})
				if e != nil || app.Status != "active" {
					return nil, resource.ErrActionInvalidated
				}
				item.Service = &configbundle.CollectionService{ID: service.ID.String(), Name: service.ServiceName, Namespace: service.ServiceNamespace, Environment: app.Environment}
			}
			result = append(result, item)
			continue
		}
		target, err := q.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: enterprise, ID: row.TargetID})
		if err != nil || row.TargetType != "monitoring_target" {
			return nil, resource.ErrActionInvalidated
		}
		result = append(result, configbundle.Collection{ID: row.ID.String(), Integration: row.IntegrationKey, TargetID: row.TargetID.String(), TargetType: row.TargetType, Identity: target.IdentityConfig, Config: row.Config, Revision: row.DesiredRevision, AssignmentEpoch: row.AssignmentEpoch, Enabled: row.DesiredEnabled, HasCredential: row.CredentialID.Valid})
	}
	return result, nil
}

func applicationEndpointSnapshots(ctx context.Context, q *db.Queries, enterprise, collector uuid.UUID) ([]configbundle.ApplicationEndpoint, error) {
	rows, err := q.ListCollectorApplicationEndpoints(ctx, db.ListCollectorApplicationEndpointsParams{EnterpriseID: enterprise, CollectorID: uuid.NullUUID{UUID: collector, Valid: true}})
	if err != nil {
		return nil, err
	}
	result := make([]configbundle.ApplicationEndpoint, 0, len(rows))
	for _, row := range rows {
		result = append(result, configbundle.ApplicationEndpoint{ID: row.ID.String(), ApplicationID: row.ApplicationID.String(), Protocol: row.Protocol, Signals: row.Signals, Version: row.Version})
	}
	return result, nil
}
