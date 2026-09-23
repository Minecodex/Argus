package component

import (
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
)

// TelemetryDomain is shared by HTTP and Agent workers. Native tools must use
// the same query engine and platform defaults as the authoritative HTTP paths.
func TelemetryDomain(cfg config.Server, store *postgres.Store, actions resource.PendingActionService, query *telemetry.GRPCQueryBackend) telemetry.Service {
	return telemetry.Service{Store: store, Access: resource.AccessService{}, Actions: actions, Query: query, Engine: query, OtelcolKubernetesImage: cfg.OtelcolKubernetesImage}
}
