package toolgateway

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func verifyCollectionMetadataActivityAndSDKCoverage(t *testing.T, ctx context.Context, store *postgres.Store, actor authorization.Actor, target, application uuid.UUID, reg *mcp.Registry) {
	t.Helper()
	domain := monitoring.Service{Store: store, CursorKey: bytes.Repeat([]byte{15}, 32)}
	reads := mcp.NewRegistry()
	if err := (MonitoringTools{Base: ResourceTools{Store: store}, Monitoring: domain, Kubernetes: kubernetesworkbench.Service{Store: store}}).Register(reads); err != nil {
		t.Fatal("current MonitoringTools dependency registration failed", err)
	}
	collection := uuid.New()
	// Controlled metadata row: deliberately healthy/enabled, so false cannot
	// accidentally mean that the target's configured collection is stopped.
	if _, err := store.Pool.Exec(ctx, `INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,desired_enabled,health,last_sample_at,created_by) VALUES($1,$2,'metadata-activity','mysql','monitoring_target',$3,true,'healthy',now(),$4)`, collection, actor.EnterpriseID, target, actor.SubjectID); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"monitoring.collection.list", "monitoring.collection.get"} {
		input := map[string]any{"search": "metadata-activity"}
		if tool == "monitoring.collection.get" {
			input = map[string]any{"id": collection.String()}
		}
		result, err := reads.Call(ctx, mcp.Call{Enterprise: actor.EnterpriseID.String(), Subject: actor.SubjectID.String(), SubjectType: actor.SubjectType, ToolID: tool, Input: input})
		if err != nil || result.Structured["sampling_performed"] != false || result.Structured["data_state"] != "not_checked" {
			t.Fatal("configuration read lost its explicit no-query activity", tool, err)
		}
	}
	capability, err := reg.Call(ctx, mcp.Call{Enterprise: actor.EnterpriseID.String(), Subject: actor.SubjectID.String(), SubjectType: actor.SubjectType, ToolID: "observe.capabilities.get", Input: map[string]any{
		"resources": []any{map[string]any{"type": "application", "id": application.String()}}, "sections": []string{"collection_state"},
	}})
	if err != nil {
		t.Fatal("SDK-only capability coverage request failed", err)
	}
	rows := capability.Structured["items"].([]any)
	section := rows[0].(map[string]any)["sections"].(map[string]any)["collection_state"].(map[string]any)
	if section["coverage"] != "collection_instances_only" || section["scope_note"] == "" || len(section["items"].([]any)) != 0 {
		t.Fatal("empty SDK-only application's collection section lacks explicit coverage")
	}
	endpoints, err := domain.IngestEndpoints(ctx, actor, application, monitoring.ListInput{Limit: 10})
	if err != nil || len(endpoints.Items) == 0 {
		t.Fatal("counterexample requires an actual configured SDK endpoint despite zero collections", err)
	}
	verifySourceVersionPagesAreNotIndependentSources(t, ctx, store, actor, application, endpoints.Items[0].ID, reg)
	verifyObservationCatalogBudgetAndUnobserved(t, store, actor)
}
