package kubernetesworkbench

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type discoveryBackend interface {
	InventoryDefinition(context.Context, db.KubernetesCluster, string) (kubernetesinventory.Definition, error)
	InventoryTypes(context.Context, db.KubernetesCluster) (kubernetesinventory.TypeCatalog, error)
}

func (s Service) definition(ctx context.Context, cluster db.KubernetesCluster, key string) (kubernetesinventory.Definition, error) {
	if value, exists := kubernetesinventory.Lookup(key); exists {
		return value, nil
	}
	backend, ok := s.Backend.(discoveryBackend)
	if !ok {
		return kubernetesinventory.Definition{}, kubernetesinventory.ErrProtocol
	}
	return backend.InventoryDefinition(ctx, cluster, key)
}
func (s Service) Types(ctx context.Context, a Actor, clusterID uuid.UUID) (kubernetesinventory.TypeCatalog, error) {
	cluster, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return kubernetesinventory.TypeCatalog{}, err
	}
	backend, ok := s.Backend.(discoveryBackend)
	if !ok {
		return kubernetesinventory.TypeCatalog{}, kubernetesinventory.ErrProtocol
	}
	result, err := backend.InventoryTypes(ctx, cluster)
	if err != nil {
		return result, err
	}
	filtered := []kubernetesinventory.Definition{}
	for _, definition := range result.Items {
		if scope.ClusterWide || definition.Namespaced || definition.Key == "namespace" {
			filtered = append(filtered, definition)
		}
	}
	result.Items = filtered
	_, _, err = s.scope(ctx, a, clusterID)
	return result, err
}
