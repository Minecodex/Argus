package kubernetesreader

import (
	"context"
	"encoding/json"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (reader Reader) InventoryDefinition(ctx context.Context, cluster db.KubernetesCluster, key string) (kubernetesinventory.Definition, error) {
	if value, exists := kubernetesinventory.Lookup(key); exists {
		return value, nil
	}
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.Definition{}, err
		}
		defer cleanup()
		return client.Definition(ctx, key)
	}
	result, err := reader.connectorDefinitions(ctx, cluster, key)
	if err != nil {
		return kubernetesinventory.Definition{}, err
	}
	if len(result.Items) != 1 || result.Items[0].Key != key {
		return kubernetesinventory.Definition{}, kubernetesinventory.ErrProtocol
	}
	return result.Items[0], nil
}
func (reader Reader) InventoryTypes(ctx context.Context, cluster db.KubernetesCluster) (kubernetesinventory.TypeCatalog, error) {
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.TypeCatalog{}, err
		}
		defer cleanup()
		return client.Types(ctx)
	}
	return reader.connectorDefinitions(ctx, cluster, "__inventory_types")
}
func (reader Reader) connectorDefinitions(ctx context.Context, cluster db.KubernetesCluster, key string) (kubernetesinventory.TypeCatalog, error) {
	out := kubernetesinventory.TypeCatalog{Items: []kubernetesinventory.Definition{}}
	request := &connectorv1.KubernetesResourceQuery{SchemaVersion: kubernetesinventory.SchemaVersion, ClusterId: cluster.ID.String(), ResourceType: key, DefinitionsOnly: true, MaxResultBytes: 1 << 20}
	var response connectorv1.KubernetesResourceQueryResult
	if err := reader.connectorCommand(ctx, cluster, "kubernetes_resource_query", request, &response); err != nil {
		return out, err
	}
	if response.SchemaVersion != kubernetesinventory.SchemaVersion || response.ResultKind != "definitions" {
		return out, kubernetesinventory.ErrProtocol
	}
	if err := kubernetesinventory.ErrorFromCode(response.ErrorCode); err != nil {
		return out, err
	}
	if len(response.ResourcesJson) > 1000 || response.Truncated {
		return out, kubernetesinventory.ErrBudget
	}
	used := 0
	for _, raw := range response.ResourcesJson {
		used += len(raw)
		if used > 1<<20 {
			return out, kubernetesinventory.ErrBudget
		}
		var definition kubernetesinventory.Definition
		if json.Unmarshal(raw, &definition) != nil || definition.Key == "" || definition.Kind == "" || definition.Version == "" || definition.Resource == "" {
			return out, kubernetesinventory.ErrProtocol
		}
		out.Items = append(out.Items, definition)
	}
	out.Complete = response.TypeCatalogComplete
	out.UnavailableReason = response.UnavailableReason
	return out, nil
}
