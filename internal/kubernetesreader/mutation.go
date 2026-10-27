package kubernetesreader

import (
	"context"
	"encoding/json"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (reader Reader) MutateInventory(ctx context.Context, cluster db.KubernetesCluster, input kubernetesinventory.Mutation, dryRun bool) (kubernetesinventory.MutationResult, error) {
	if cluster.ConnectionMode == "direct" {
		if input.PrivateInputID != "" {
			recipient := "worker"
			if dryRun {
				recipient = "server"
			}
			values, err := reader.Secrets.LoadKubernetesInput(ctx, cluster.EnterpriseID, cluster.ID, input, dryRun, recipient, cluster.ID.String())
			if err != nil {
				return kubernetesinventory.MutationResult{}, err
			}
			defer clear(values)
			input, err = kubernetesinventory.WithSecretValues(input, values)
			if err != nil {
				return kubernetesinventory.MutationResult{}, err
			}
			defer clear(input.Object)
		}
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.MutationResult{}, err
		}
		defer cleanup()
		return client.Mutate(ctx, input, dryRun)
	}
	return reader.connectorMutation(ctx, cluster, input, dryRun, false, false, "")
}
func (reader Reader) ObserveInventoryMutation(ctx context.Context, cluster db.KubernetesCluster, input kubernetesinventory.Mutation, digest string) (kubernetesinventory.MutationResult, error) {
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.MutationResult{}, err
		}
		defer cleanup()
		return client.ReadMutationOutcome(ctx, input, digest)
	}
	return reader.connectorMutation(ctx, cluster, input, false, true, false, digest)
}
func (reader Reader) AdvanceInventoryDrain(ctx context.Context, cluster db.KubernetesCluster, input kubernetesinventory.Mutation) (kubernetesinventory.MutationResult, error) {
	if input.Operation != "drain" {
		return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrMutationInvalid
	}
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.MutationResult{}, err
		}
		defer cleanup()
		return client.AdvanceDrain(ctx, input)
	}
	return reader.connectorMutation(ctx, cluster, input, false, false, true, "")
}
func (reader Reader) connectorMutation(ctx context.Context, cluster db.KubernetesCluster, input kubernetesinventory.Mutation, dryRun, observe, advance bool, digest string) (kubernetesinventory.MutationResult, error) {
	definition, err := reader.InventoryDefinition(ctx, cluster, input.Query.Type)
	if err != nil {
		return kubernetesinventory.MutationResult{}, err
	}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > 1<<20 {
		return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrMutationInvalid
	}
	request := &connectorv1.KubernetesResourceMutation{SchemaVersion: kubernetesinventory.MutationSchemaVersion, ClusterId: cluster.ID.String(), OperationId: input.OperationRef, MutationJson: raw, DryRun: dryRun, ObserveOnly: observe, AdvanceDrain: advance, ExpectedDigest: digest}
	var response connectorv1.KubernetesResourceMutationResult
	if err = reader.connectorCommand(ctx, cluster, "kubernetes_resource_mutation", request, &response); err != nil {
		if !dryRun && !observe {
			return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrMutationUnknown
		}
		return kubernetesinventory.MutationResult{}, err
	}
	if response.SchemaVersion != kubernetesinventory.MutationSchemaVersion {
		return kubernetesinventory.MutationResult{}, kubernetesinventory.ErrProtocol
	}
	if err = kubernetesinventory.ErrorFromCode(response.ErrorCode); err != nil {
		return kubernetesinventory.MutationResult{}, err
	}
	var result kubernetesinventory.MutationResult
	if len(response.ResultJson) > 2<<20 || json.Unmarshal(response.ResultJson, &result) != nil {
		return result, kubernetesinventory.ErrProtocol
	}
	return kubernetesinventory.ProjectMutationResult(definition, input, result, dryRun)
}
