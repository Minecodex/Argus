package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type mutationOutcomeReader interface {
	GetInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.Object, error)
}

// A later approved edit replaces the mutation marker. Once the API has already
// acknowledged our write, the same object's newer generation proves that its
// original desired state was superseded. Unknown writes remain ambiguous.
func supersededObservation(ctx context.Context, backend mutationOutcomeReader, cluster db.KubernetesCluster, input kubernetesinventory.Mutation, accepted json.RawMessage) (*kubernetesinventory.Object, bool) {
	var previous struct {
		Accepted   bool   `json:"accepted"`
		UID        string `json:"uid"`
		Generation int64  `json:"generation"`
	}
	if json.Unmarshal(accepted, &previous) != nil || !previous.Accepted || previous.UID == "" || previous.Generation <= 0 || input.Operation == "delete" || input.Operation == "drain" {
		return nil, false
	}
	query := input.Query
	query.UID = previous.UID
	current, err := backend.GetInventory(ctx, cluster, query)
	if err != nil || current.UID != previous.UID || current.Name != query.Name || current.Namespace != query.Namespace || current.NamespaceUID != query.NamespaceUID || current.Definition.Key != query.Type || current.Generation <= previous.Generation {
		return nil, false
	}
	return &current, true
}
