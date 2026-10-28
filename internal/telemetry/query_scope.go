package telemetry

import (
	"encoding/hex"
	"github.com/google/uuid"
	commonv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/common/v1"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/resourceref"
)

const QueryProtocolVersion = "argus.telemetry_query/v6"
const CatalogProtocolVersion = "argus.telemetry_catalog/v5"

func queryProtoScope(enterprise, subject uuid.UUID, kind string, refs []resourceref.Ref, ids []uuid.UUID, version int64, signal string, sources []string, signature string) (*telemetryv1.TelemetryQueryScope, error) {
	resources, err := resourceref.Normalize(refs)
	for _, ref := range resources {
		if !resourceref.ValidTelemetryRoot(ref.Type) {
			return nil, ErrQueryScope
		}
	}
	if err != nil || !resourceref.MatchIDs(resources, ids) || enterprise == uuid.Nil || subject == uuid.Nil || kind != "user" && kind != "service_account" || version < 1 || len(resources) == 0 || len(sources) == 0 || !validScopeSignature(signature) {
		return nil, ErrQueryScope
	}
	result := &telemetryv1.TelemetryQueryScope{EnterpriseId: enterprise.String(), SubjectId: subject.String(), SubjectType: kind, Signal: signal, AuthorizationVersion: uint64(version), SourceKeys: sources, ScopeSignature: signature}
	for _, ref := range resources {
		result.AuthorizedResources = append(result.AuthorizedResources, &commonv1.ResourceRef{ResourceType: ref.Type, ResourceId: ref.ID.String()})
	}
	result.ScopeHash = scopeHash(enterprise, subject, kind, resources, version, signal, signature, sources...)
	if _, _, err := scopeFromProto(result, signal); err != nil {
		return nil, err
	}
	return result, nil
}
func validScopeSignature(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func scopeRefsFromProto(value *telemetryv1.TelemetryQueryScope) ([]resourceref.Ref, error) {
	if value == nil {
		return nil, ErrQueryScope
	}
	refs := make([]resourceref.Ref, 0, len(value.AuthorizedResources))
	for _, r := range value.AuthorizedResources {
		if r == nil {
			return nil, ErrQueryScope
		}
		id, err := uuid.Parse(r.ResourceId)
		if err != nil {
			return nil, ErrQueryScope
		}
		refs = append(refs, resourceref.Ref{Type: r.ResourceType, ID: id})
	}
	refs, err := resourceref.Normalize(refs)
	if err != nil || len(refs) != len(value.AuthorizedResources) {
		return nil, ErrQueryScope
	}
	for _, ref := range refs {
		if !resourceref.ValidTelemetryRoot(ref.Type) {
			return nil, ErrQueryScope
		}
	}
	return refs, nil
}
