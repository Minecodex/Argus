package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/gen/openapi/telemetryapi"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
)

type CollectorPreviewTools struct {
	Base    ResourceTools
	Service telemetry.Service
}

func (tools CollectorPreviewTools) Register(registry *mcp.Registry) error {
	document, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	for _, target := range []struct{ prefix, kind, id string }{{"host", "host", "host_id"}, {"kubernetes", "kubernetes_cluster", "cluster_id"}} {
		for _, operation := range []string{"install", "configure", "upgrade", "repair", "uninstall"} {
			item := nativePreview{id: target.prefix + ".collector." + operation + ".preview", permission: "telemetry.collector.manage", schema: "CollectorPreview", idField: target.id,
				run: func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
					input, err := collectorInput(in)
					if err != nil {
						return db.PendingAction{}, err
					}
					return tools.Service.PreviewCollectorAction(ctx, previewTelemetryActor(s, e), target.kind, id, operation, input, key)
				}}
			if err := registerNativePreview(registry, tools.Base, document, item); err != nil {
				return err
			}
		}
	}
	item := nativePreview{id: "kubernetes.node_host_binding.confirm.preview", permission: "telemetry.collector.manage", schema: "NodeHostBindingPreview", idField: "binding_id",
		run: func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			host, err := uuid.Parse(stringValue(in, "host_id"))
			if err != nil {
				return db.PendingAction{}, err
			}
			return tools.Service.PreviewBinding(ctx, previewTelemetryActor(s, e), id, host, intValue(in, "expected_version"), key)
		}}
	if err := registerNativePreview(registry, tools.Base, document, item); err != nil {
		return err
	}
	return registry.ValidatePairs()
}

func previewTelemetryActor(subject resource.Subject, enterprise uuid.UUID) telemetry.Actor {
	return telemetry.Actor{RunID: subject.RunID, EnterpriseID: enterprise, SubjectID: uuid.MustParse(subject.ActorID), AuthorizationVersion: subject.AuthorizationVersion, AuthorizedResourceIDs: subject.AuthorizedResourceIDs}
}

func collectorInput(input map[string]any) (telemetry.CollectorPreviewInput, error) {
	// Decode the public wire representation into the same shape used by HTTP.
	data, err := json.Marshal(input)
	if err != nil {
		return telemetry.CollectorPreviewInput{}, err
	}
	var body telemetryapi.CollectorPreview
	if err := json.Unmarshal(data, &body); err != nil {
		return telemetry.CollectorPreviewInput{}, err
	}
	profiles := make([]uuid.UUID, len(body.ProfileIds))
	for i, id := range body.ProfileIds {
		profiles[i] = uuid.UUID(id)
	}
	value := telemetry.CollectorPreviewInput{DistributionVersionID: uuid.UUID(body.DistributionVersionId), ProfileIDs: profiles, RouteKind: string(body.RouteKind), Transport: string(body.Transport)}
	if body.GatewayCollectorId != nil {
		value.GatewayCollectorID = uuid.NullUUID{UUID: uuid.UUID(*body.GatewayCollectorId), Valid: true}
	}
	if body.ExpectedVersion != nil {
		value.ExpectedVersion = *body.ExpectedVersion
	}
	if body.KubernetesImage != nil {
		value.KubernetesImage = *body.KubernetesImage
	}
	if body.ImagePullSecrets != nil {
		value.ImagePullSecrets = append([]string{}, (*body.ImagePullSecrets)...)
	}
	if body.LoopbackPort != nil {
		if *body.LoopbackPort < 1 || *body.LoopbackPort >= 65535 {
			return value, fmt.Errorf("invalid loopback port")
		}
		value.LoopbackPort = int32(*body.LoopbackPort)
	}
	return value, nil
}
