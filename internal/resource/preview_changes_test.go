package resource

import (
	"encoding/json"
	"os"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestHostPublicChangesMatchPartialUpdateAndPreserveSystemLabels(t *testing.T) {
	changes := hostUpdateChanges("old host", "unchanged.internal", "production", "10.0.0.1", 22,
		[]byte(`{"env":"prod","argus.io/source":"collector"}`), HostInput{Name: "new host <>&", Labels: map[string]string{}})
	if len(changes) != 2 || changes[0].Path != "labels" || changes[1].Path != "name" || changes[1].Before != "old host" || changes[1].After != "new host <>&" {
		t.Fatalf("wrong frozen partial update: %#v", changes)
	}
	if changes[0].After != `{"argus.io/source":"collector"}` {
		t.Fatalf("label clearing must retain system labels: %v", changes[0].After)
	}
}

func TestPublicChangesExcludePrivateFieldsAndUnchangedValues(t *testing.T) {
	changes := PublicChanges(map[string]any{"name": "same", "credential_id": "old", "password": "old"}, map[string]any{"name": "same", "credential_id": "new", "password": "new", "authentication_changed": true})
	if len(changes) != 1 || changes[0].Path != "authentication_changed" {
		t.Fatalf("authority or unchanged field leaked: %#v", changes)
	}
}

func TestPublicChangesRespectPublicJSONContractWithNamespacedLabels(t *testing.T) {
	raw, err := os.ReadFile("../../api/schemas/common/public-json.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const id = "https://argus.io/schemas/v1/common/public-json.schema.json"
	if err := compiler.AddResource(id, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(id + "#/$defs/PublicJsonObject")
	if err != nil {
		t.Fatal(err)
	}
	changes := PublicChanges(map[string]any{"labels": map[string]string{"argus.io/source": "collector", "team.name": "ops"}}, map[string]any{"labels": map[string]string{}, "authentication_changed": true})
	encoded, _ := json.Marshal(map[string]any{"changes": changes})
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("invalid public field differences: %v", err)
	}
}
