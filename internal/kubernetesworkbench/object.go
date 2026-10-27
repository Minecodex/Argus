package kubernetesworkbench

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

type ObjectInput struct {
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	UID         string    `json:"uid"`
	NamespaceID uuid.UUID `json:"namespace_id"`
}

func (s Service) Get(ctx context.Context, a Actor, clusterID uuid.UUID, in ObjectInput) (kubernetesinventory.Object, error) {
	if in.Name == "" || in.UID == "" {
		return kubernetesinventory.Object{}, kubernetesinventory.ErrInvalid
	}
	cluster, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	definition, err := s.definition(ctx, cluster, in.Type)
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	query := kubernetesinventory.Query{Type: in.Type, Name: in.Name, UID: in.UID}
	if definition.Namespaced {
		if in.NamespaceID == uuid.Nil {
			return kubernetesinventory.Object{}, kubernetesinventory.ErrInvalid
		}
		selected, _, e := s.selectedNamespaces(ctx, a, scope, Selection{Mode: "explicit", IDs: []uuid.UUID{in.NamespaceID}})
		if e != nil {
			return kubernetesinventory.Object{}, e
		}
		query.Namespace, query.NamespaceUID = selected[0].Name, selected[0].UID
	} else {
		if in.NamespaceID != uuid.Nil {
			return kubernetesinventory.Object{}, kubernetesinventory.ErrInvalid
		}
		if !scope.ClusterWide && (in.Type != "namespace" || !slices.ContainsFunc(scope.Namespaces, func(n Namespace) bool { return n.Name == in.Name && n.UID == in.UID })) {
			return kubernetesinventory.Object{}, authorization.ErrAccessDenied
		}
	}
	object, err := s.Backend.GetInventory(ctx, cluster, query)
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	if object.Definition != definition || object.UID != in.UID || object.Name != in.Name || object.Namespace != query.Namespace || definition.Namespaced && object.NamespaceUID != query.NamespaceUID {
		return kubernetesinventory.Object{}, kubernetesinventory.ErrIdentityChanged
	}
	_, current, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	if scopeSignature(scope, scope.Namespaces) != scopeSignature(current, current.Namespaces) {
		return kubernetesinventory.Object{}, authorization.ErrAccessDenied
	}
	return object, nil
}
