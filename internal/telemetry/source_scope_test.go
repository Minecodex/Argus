package telemetry

import (
	"testing"

	"github.com/google/uuid"
	commonv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/common/v1"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
)

func TestRPCScopeHashBindsSourceRevisions(t *testing.T) {
	enterprise, resource, source, subject := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	keys := []string{source.String() + ":1"}
	scope := &telemetryv1.TelemetryQueryScope{EnterpriseId: enterprise.String(), AuthorizedResources: []*commonv1.ResourceRef{{ResourceType: "host", ResourceId: resource.String()}}, SubjectId: subject.String(), SubjectType: "user", Signal: "logs", AuthorizationVersion: 1, SourceKeys: keys, ScopeHash: scopeHash(enterprise, subject, "user", []uuid.UUID{resource}, 1, "logs", keys...)}
	if _, _, err := scopeFromProto(scope, "logs"); err != nil {
		t.Fatal(err)
	}
	scope.SourceKeys = []string{source.String() + ":2"}
	if _, _, err := scopeFromProto(scope, "logs"); err == nil {
		t.Fatal("changing only the source revision preserved the scope fingerprint")
	}
}

func TestRPCScopeSeparatesSubjectsAndRejectsObjectTypeConfusion(t *testing.T) {
	enterprise, resource, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	a := scopeHash(enterprise, first, "user", []uuid.UUID{resource}, 1, "logs")
	if a == scopeHash(enterprise, second, "user", []uuid.UUID{resource}, 1, "logs") || a == scopeHash(enterprise, first, "service_account", []uuid.UUID{resource}, 1, "logs") {
		t.Fatal("scope hash ignores subject identity")
	}
	scope := queryProtoScope(enterprise, first, "user", []*commonv1.ResourceRef{{ResourceType: "host", ResourceId: resource.String()}}, []uuid.UUID{resource}, 1, "logs", nil)
	scope.SubjectId = second.String()
	if _, _, err := scopeFromProto(scope, "logs"); err == nil {
		t.Fatal("subject replaced without invalidating scope")
	}
	scope.SubjectId = first.String()
	scope.AuthorizedResources[0].ResourceType = "dashboard"
	if _, _, err := scopeFromProto(scope, "logs"); err == nil {
		t.Fatal("Dashboard ID accepted as telemetry resource")
	}
	scope.AuthorizedResources[0].ResourceType = "host"
	scope.SubjectId = ""
	if _, _, err := scopeFromProto(scope, "logs"); err == nil {
		t.Fatal("anonymous scope accepted")
	}
}
