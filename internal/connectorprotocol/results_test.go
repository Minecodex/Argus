package connectorprotocol

import (
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"testing"
)

func TestKubernetesMutationRunningAndResultContract(t *testing.T) {
	message, err := Result("kubernetes_resource_mutation")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := message.(*connectorv1.KubernetesResourceMutationResult); !ok {
		t.Fatal("wrong running/result factory")
	}
	encoded, err := anypb.New(message)
	if err != nil {
		t.Fatal(err)
	}
	if !ResultAllowed("kubernetes_resource_mutation", encoded) || ResultAllowed("kubernetes_resource_query", encoded) {
		t.Fatal("mutation result routing")
	}
}
