package telemetry

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
)

func TestNamespaceSelectionMustResolveBeforeQueryRPC(t *testing.T) {
	namespace, application, source, enterprise, user := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	snapshot := observationscope.Snapshot{Resources: []observationscope.Resource{{Ref: observationscope.Ref{Kind: "kubernetes_namespace", ID: namespace}}}, Signature: testScopeSignature, Query: &observationscope.SourceResult{DataResources: []resourceref.Ref{{Type: "application", ID: application}}, Keys: []string{source.String() + ":1"}, Signature: testScopeSignature}}
	if _, err := queryProtoScope(enterprise, user, "user", snapshot.Refs(), snapshot.IDs(), 1, "logs", snapshot.Query.Keys, snapshot.QuerySignature()); err == nil {
		t.Fatal("virtual Namespace reached the physical query protocol")
	}
	request, err := queryProtoScope(enterprise, user, "user", snapshot.DataRefs(), snapshot.DataIDs(), 1, "logs", snapshot.Query.Keys, snapshot.QuerySignature())
	if err != nil || len(request.AuthorizedResources) != 1 || request.AuthorizedResources[0].ResourceType != "application" || request.AuthorizedResources[0].ResourceId != application.String() {
		t.Fatalf("bounded physical scope: %v", err)
	}
}
