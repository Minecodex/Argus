package contract_test

import "testing"

func TestWorkspaceProfileIsAcceptedByPublicContract(t *testing.T) {
	compiler := openAPISchemaCompiler(t, repoRoot(t))
	schema, err := compiler.Compile("https://argus.io/openapi/v1/argus.bundle.json#/components/schemas/SandboxProfileWrite")
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"name": "Offline business analysis", "backend_id": "11111111-1111-4111-8111-111111111111", "image_id": "22222222-2222-4222-8222-222222222222",
		"task_kinds": []any{"agent_workspace"}, "cpu_millis": 500, "memory_mib": 512, "timeout_seconds": 300, "network_mode": "none", "status": "enabled", "expected_version": 0,
	}
	if err := schema.Validate(input); err != nil {
		t.Fatalf("Workspace profile rejected by public contract: %v", err)
	}
	input["task_kinds"] = []any{"attachment_processing"}
	if err := schema.Validate(input); err == nil {
		t.Fatal("removed task kind remains configurable")
	}
}
