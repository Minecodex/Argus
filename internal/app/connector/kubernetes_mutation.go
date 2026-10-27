package connector

import (
	"context"
	"encoding/json"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/client-go/dynamic"
)

func executeKubernetesMutation(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.KubernetesResourceMutationResult, error) {
	result := &connectorv1.KubernetesResourceMutationResult{SchemaVersion: kubernetesinventory.MutationSchemaVersion}
	failure := func(err error) (*connectorv1.KubernetesResourceMutationResult, error) {
		result.ErrorCode = kubernetesinventory.ErrorCode(err)
		return result, nil
	}
	var request connectorv1.KubernetesResourceMutation
	defer func() { clear(request.PrivateSecretValuesJson) }()
	if payload.UnmarshalTo(&request) != nil {
		return failure(kubernetesinventory.ErrMutationInvalid)
	}
	if request.SchemaVersion != kubernetesinventory.MutationSchemaVersion {
		return failure(kubernetesinventory.ErrProtocol)
	}
	if request.ClusterId == "" || request.OperationId == "" || len(request.MutationJson) > 1<<20 || request.DryRun && request.ObserveOnly || request.AdvanceDrain && (request.DryRun || request.ObserveOnly) {
		return failure(kubernetesinventory.ErrMutationInvalid)
	}
	var input kubernetesinventory.Mutation
	if json.Unmarshal(request.MutationJson, &input) != nil || input.OperationRef != request.OperationId {
		return failure(kubernetesinventory.ErrMutationInvalid)
	}
	if !request.ObserveOnly && input.PrivateInputID != "" {
		var err error
		input, err = kubernetesinventory.WithSecretValues(input, request.PrivateSecretValuesJson)
		if err != nil {
			return failure(err)
		}
		defer clear(input.Object)
	} else if len(request.PrivateSecretValuesJson) > 0 {
		return failure(kubernetesinventory.ErrMutationInvalid)
	}
	config, err := connectorKubeconfig(credential, "")
	if err != nil {
		return failure(kubernetesinventory.ErrUnavailable)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return failure(kubernetesinventory.ErrUnavailable)
	}
	engine := kubernetesinventory.Client{Dynamic: client}
	var value kubernetesinventory.MutationResult
	if request.AdvanceDrain {
		value, err = engine.AdvanceDrain(ctx, input)
	} else if request.ObserveOnly {
		value, err = engine.ReadMutationOutcome(ctx, input, request.ExpectedDigest)
	} else {
		value, err = engine.Mutate(ctx, input, request.DryRun)
	}
	if err != nil {
		return failure(err)
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 2<<20 {
		return failure(kubernetesinventory.ErrBudget)
	}
	result.ResultJson = raw
	return result, nil
}
