package toolgateway

import (
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"slices"
	"testing"
)

func TestResourceToolCatalogIsStrictAndComplete(t *testing.T) {
	t.Parallel()
	registry := mcp.NewRegistry()
	if err := (ResourceTools{Store: &postgres.Store{}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"kubernetes.namespace.list", "kubernetes.node.list", "kubernetes.pod.list", "kubernetes.deployment.list",
		"kubernetes.statefulset.list", "kubernetes.daemonset.list", "kubernetes.service.list", "kubernetes.pod.logs",
	}
	ids := make([]string, 0)
	for _, metadata := range registry.ModelCatalog() {
		ids = append(ids, metadata.ID)
		if metadata.InputSchema == nil || metadata.InputSchema["additionalProperties"] != false {
			t.Fatalf("tool %s has a permissive schema: %#v", metadata.ID, metadata.InputSchema)
		}
	}
	for _, id := range want {
		if !slices.Contains(ids, id) {
			t.Errorf("model catalog lacks %s", id)
		}
	}
	cancel, ok := registry.Lookup("pending_action.cancel")
	if !ok || cancel.Risk != "write" || !slices.Contains(cancel.Required, "pending_action.confirm") {
		t.Fatalf("pending_action.cancel metadata = %#v", cancel)
	}
	logs, ok := registry.Lookup("kubernetes.pod.logs")
	if !ok || !slices.Contains(logs.Required, "kubernetes.logs") || logs.Validate(map[string]any{"cluster_id": "bad", "namespace": "ns", "pod": "pod"}) == nil {
		t.Fatalf("pod logs metadata or validation is incomplete: %#v", logs)
	}
}

func TestToolIdempotencyIsBoundToTrustedInvocationAndCall(t *testing.T) {
	t.Parallel()
	base := mcp.Call{ToolID: "host.update.preview", RunID: "run-1", InvocationID: "run-1", CallID: "call-1", Subject: "user-1", Input: map[string]any{"request_id": "model-controlled"}}
	if idempotency(base) != idempotency(base) {
		t.Fatal("the same Tool invocation did not produce a stable idempotency key")
	}
	otherInvocation := base
	otherInvocation.RunID = ""
	otherInvocation.InvocationID = "service-run-2"
	if idempotency(base) == idempotency(otherInvocation) {
		t.Fatal("different invocations shared a Tool idempotency key")
	}
	otherCall := base
	otherCall.CallID = "call-2"
	if idempotency(base) == idempotency(otherCall) {
		t.Fatal("different ToolCalls in one Run shared an idempotency key")
	}
	changedRequest := base
	changedRequest.Input = map[string]any{"request_id": "different-model-controlled-value"}
	if idempotency(base) != idempotency(changedRequest) {
		t.Fatal("model-controlled request_id changed the internal idempotency identity")
	}
	legacyRun := base
	legacyRun.InvocationID = ""
	if idempotency(base) != idempotency(legacyRun) {
		t.Fatal("Agent RunID fallback changed the internal idempotency identity")
	}
}

func TestPendingActionGetUsesThePublicPreviewOutputVersion(t *testing.T) {
	t.Parallel()
	registry := mcp.NewRegistry()
	tools := ResourceTools{Store: &postgres.Store{}}
	if err := tools.Register(registry); err != nil {
		t.Fatal(err)
	}
	get, ok := registry.Lookup("pending_action.get")
	if !ok {
		t.Fatal("pending_action.get is not registered")
	}
	preview, ok := registry.Lookup("host.update.preview")
	if !ok {
		t.Fatal("host.update.preview is not registered")
	}
	if get.OutputVersion != "argus.pending_action/v1" || preview.OutputVersion != get.OutputVersion {
		t.Fatalf("PendingAction output versions differ: get=%q preview=%q", get.OutputVersion, preview.OutputVersion)
	}
	if get.OutputSchemaHash == "" || preview.OutputSchemaHash != get.OutputSchemaHash {
		t.Fatalf("PendingAction output Schema hashes differ: get=%q preview=%q", get.OutputSchemaHash, preview.OutputSchemaHash)
	}
}
