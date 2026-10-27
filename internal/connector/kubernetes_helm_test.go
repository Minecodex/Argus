package connector

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestHelmCommandsRejectPersistedSecretsAndProtocolModeConfusion(t *testing.T) {
	cluster := uuid.NewString()
	id := uuid.NewString()
	raw, _ := json.Marshal(kuberneteshelm.Request{SchemaVersion: kuberneteshelm.SchemaVersion, OperationID: id, Operation: "history"})
	for _, mode := range []string{"valid", "private", "version", "legacy", "nested_private", "operation", "cluster", "query_write", "mode"} {
		t.Run(mode, func(t *testing.T) {
			wire := connectorv1.KubernetesHelmCommand{SchemaVersion: kuberneteshelm.SchemaVersion, ClusterId: cluster, OperationId: id, Mode: "query", RequestJson: raw}
			switch mode {
			case "private":
				wire.PrivateMaterialJson = []byte(`{"chart":"must-not-persist"}`)
			case "version":
				wire.SchemaVersion = "argus.kubernetes_helm/v0"
			case "legacy":
				wire.SchemaVersion = "argus.kubernetes_helm/v1"
			case "nested_private":
				var input map[string]any
				_ = json.Unmarshal(raw, &input)
				input["source_credentials"] = map[string]string{"password": "private-cannot-persist"}
				wire.RequestJson, _ = json.Marshal(input)
			case "operation":
				wire.OperationId = uuid.NewString()
			case "cluster":
				wire.ClusterId = uuid.NewString()
			case "query_write":
				wire.RequestJson, _ = json.Marshal(kuberneteshelm.Request{SchemaVersion: kuberneteshelm.SchemaVersion, OperationID: id, Operation: "uninstall"})
			case "mode":
				wire.Mode = "shell"
			}
			payload, _ := json.Marshal(wire)
			_, _, _, _, e := helmRequest(db.ConnectorCommand{OperationRef: cluster, Payload: payload})
			if (mode == "valid") != (e == nil) {
				t.Fatal(mode, e)
			}
		})
	}
}
