package remoteexecution

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func resolveTarget(ctx context.Context, q *db.Queries, enterprise uuid.UUID, target Target) (Target, error) {
	if target.ResourceType == "host" {
		h, err := q.GetHost(ctx, db.GetHostParams{ID: target.ResourceID, EnterpriseID: enterprise})
		if err != nil || h.Status != "active" {
			return target, Error("REMOTE_RESOURCE_FORBIDDEN")
		}
		target.Name = h.Name
		target.ResourceVersion = h.ResourceVersion
	} else {
		c, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: target.ResourceID, EnterpriseID: enterprise})
		if err != nil || c.Status != "active" {
			return target, Error("REMOTE_RESOURCE_FORBIDDEN")
		}
		target.Name = c.Name
		target.ResourceVersion = c.ResourceVersion
	}
	return target, nil
}
func targetScope(target Target) Target { target.Name = ""; target.ResourceVersion = 0; return target }
