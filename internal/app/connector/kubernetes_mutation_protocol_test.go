package connector

import (
	"testing"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestKubernetesMutationRejectsOldProtocolBeforeAPIExecution(t *testing.T) {
	for _, version := range []string{"", "argus.kubernetes_mutation/v1", "argus.kubernetes_mutation/v2", "argus.kubernetes_mutation/v3", "argus.kubernetes_mutation/v4", "argus.kubernetes_mutation/v5"} {
		t.Run(version, func(t *testing.T) {
			payload, err := anypb.New(&connectorv1.KubernetesResourceMutation{SchemaVersion: version, ClusterId: "cluster", OperationId: "operation", MutationJson: []byte(`{"operation":"rollback"}`)})
			if err != nil {
				t.Fatal(err)
			}
			result, err := executeKubernetesMutation(t.Context(), payload, nil)
			if err != nil || result.ErrorCode != kubernetesinventory.ErrProtocol.Error() || result.SchemaVersion != kubernetesinventory.MutationSchemaVersion {
				t.Fatalf("unsupported version was not explicitly rejected before credentials/API access: %v %+v", err, result)
			}
		})
	}
}

func TestKubernetesDrainRejectsMixedReadAndWritePhases(t *testing.T) {
	for _, request := range []*connectorv1.KubernetesResourceMutation{
		{DryRun: true, AdvanceDrain: true}, {ObserveOnly: true, AdvanceDrain: true}, {DryRun: true, ObserveOnly: true},
	} {
		request.SchemaVersion = kubernetesinventory.MutationSchemaVersion
		request.ClusterId = "cluster"
		request.OperationId = "operation"
		request.MutationJson = []byte(`{"operation":"drain"}`)
		payload, err := anypb.New(request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := executeKubernetesMutation(t.Context(), payload, nil)
		if err != nil || result.ErrorCode != kubernetesinventory.ErrMutationInvalid.Error() {
			t.Fatalf("mixed phase reached API/credential handling: %+v %v", result, err)
		}
	}
}
