package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"testing"

	"github.com/google/uuid"
	commonv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/common/v1"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const testScopeSignature = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testResourceRefs(id uuid.UUID) []resourceref.Ref {
	return []resourceref.Ref{{Type: "host", ID: id}}
}

func TestTypedRPCRejectsMissingSourcesAndOlderContracts(t *testing.T) {
	enterprise, subject, resource := uuid.New(), uuid.New(), uuid.New()
	for _, kind := range []string{"host", "kubernetes_cluster", "application", "monitoring_target"} {
		scope, err := queryProtoScope(enterprise, subject, "user", []resourceref.Ref{{Type: kind, ID: resource}}, []uuid.UUID{resource}, 1, "logs", []string{uuid.NewString() + ":1"}, testScopeSignature)
		if err != nil {
			t.Fatalf("supported root %s: %v", kind, err)
		}
		scope.ScopeSignature = strings.Repeat("b", 64)
		if _, _, err = scopeFromProto(scope, "logs"); err == nil {
			t.Fatal("changed relationship signature accepted")
		}
	}
	if _, err := queryProtoScope(enterprise, subject, "user", testResourceRefs(resource), []uuid.UUID{resource}, 1, "logs", nil, testScopeSignature); err == nil {
		t.Fatal("empty sources became unrestricted")
	}
	scope, err := queryProtoScope(enterprise, subject, "user", testResourceRefs(resource), []uuid.UUID{resource}, 1, "logs", []string{uuid.NewString() + ":1"}, testScopeSignature)
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}}})
	engine := &cacheRPCEngine{}
	server := &QueryRPCServer{Limiter: &cacheRPCLimiter{}, Coordinator: &queryengine.Coordinator{KQL: engine}}
	_, err = server.ExecuteQueryV2(ctx, &telemetryv1.ExecuteQueryV2Request{SchemaVersion: "argus.telemetry_query/v4", Scope: scope, Budget: &telemetryv1.QueryBudget{MaxRows: 10}, Query: &telemetryv1.ExecuteQueryV2Request_Kql{Kql: &telemetryv1.KQLQuery{Expression: "*"}}})
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "QUERY_PROTOCOL_VERSION_UNSUPPORTED" || engine.calls != 0 {
		t.Fatalf("old contract was not rejected before execution: %v", err)
	}
}

func TestRPCScopeHashBindsSourceRevisions(t *testing.T) {
	enterprise, resource, source, subject := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	keys := []string{source.String() + ":1"}
	scope := &telemetryv1.TelemetryQueryScope{EnterpriseId: enterprise.String(), AuthorizedResources: []*commonv1.ResourceRef{{ResourceType: "host", ResourceId: resource.String()}}, SubjectId: subject.String(), SubjectType: "user", Signal: "logs", AuthorizationVersion: 1, SourceKeys: keys, ScopeSignature: testScopeSignature, ScopeHash: scopeHash(enterprise, subject, "user", testResourceRefs(resource), 1, "logs", testScopeSignature, keys...)}
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
	a := scopeHash(enterprise, first, "user", testResourceRefs(resource), 1, "logs", testScopeSignature)
	if a == scopeHash(enterprise, second, "user", testResourceRefs(resource), 1, "logs", testScopeSignature) || a == scopeHash(enterprise, first, "service_account", testResourceRefs(resource), 1, "logs", testScopeSignature) {
		t.Fatal("scope hash ignores subject identity")
	}
	scope, err := queryProtoScope(enterprise, first, "user", testResourceRefs(resource), []uuid.UUID{resource}, 1, "logs", []string{uuid.NewString() + ":1"}, testScopeSignature)
	if err != nil {
		t.Fatal(err)
	}
	scope.SubjectId = second.String()
	if _, _, err := scopeFromProto(scope, "logs"); err == nil {
		t.Fatal("subject replaced without invalidating scope")
	}
	scope.SubjectId = first.String()
	scope.AuthorizedResources[0].ResourceType = "application"
	if _, _, err := scopeFromProto(scope, "logs"); err == nil {
		t.Fatal("allowed resource kind relabeled without invalidating its scope")
	}
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
