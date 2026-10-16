package resource

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/hostinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// GetHostInventory shares the authorized persisted observation with resource
// tools. Reading never triggers collection or bypasses the Host grant.
func (service Service) GetHostInventory(ctx context.Context, enterpriseID, hostID uuid.UUID, authorizedIDs []uuid.UUID) (*hostinventory.Snapshot, error) {
	if _, err := service.GetHost(ctx, enterpriseID, hostID, authorizedIDs); err != nil {
		return nil, err
	}
	observation, err := service.Store.Queries.GetHostRuntimeObservation(ctx, db.GetHostRuntimeObservationParams{EnterpriseID: enterpriseID, HostID: hostID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return hostinventory.Decode(observation.InventoryJson)
}
