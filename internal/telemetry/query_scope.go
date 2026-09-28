package telemetry

import (
	"github.com/google/uuid"
	commonv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/common/v1"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
)

func queryProtoScope(enterprise, subject uuid.UUID, kind string, resources []*commonv1.ResourceRef, ids []uuid.UUID, version int64, signal string, sources []string) *telemetryv1.TelemetryQueryScope {
	if kind == "" {
		kind = "user"
	}
	return &telemetryv1.TelemetryQueryScope{EnterpriseId: enterprise.String(), SubjectId: subject.String(), SubjectType: kind, Signal: signal, AuthorizedResources: resources, AuthorizationVersion: uint64(version), SourceKeys: sources, ScopeHash: scopeHash(enterprise, subject, kind, ids, version, signal, sources...)}
}
