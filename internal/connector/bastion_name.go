package connector

import (
	"context"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// NameAvailable checks both the Scope and root Host names that creation claims.
func (service BastionService) NameAvailable(ctx context.Context, enterpriseID uuid.UUID, name string) (bool, error) {
	name, err := resource.NormalizeResourceName(name)
	if err != nil {
		return false, err
	}
	available, err := service.Store.Queries.BastionNameAvailable(ctx, db.BastionNameAvailableParams{EnterpriseID: enterpriseID, Name: name})
	return available.Valid && available.Bool, err
}
