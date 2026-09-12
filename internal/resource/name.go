package resource

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var (
	ErrInvalidResourceName  = errors.New("invalid resource name")
	ErrResourceNameConflict = errors.New("resource name already belongs to a live resource")
)

// NormalizeResourceName keeps creation and availability checks on the same
// name. Case is preserved for display; PostgreSQL owns case-insensitive lookup.
func NormalizeResourceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return "", ErrInvalidResourceName
	}
	return name, nil
}

func (service Service) HostNameAvailable(ctx context.Context, enterpriseID uuid.UUID, name string) (bool, error) {
	name, err := NormalizeResourceName(name)
	if err != nil {
		return false, err
	}
	available, err := service.Store.Queries.HostNameAvailable(ctx, db.HostNameAvailableParams{EnterpriseID: enterpriseID, Name: name})
	return available.Valid && available.Bool, err
}
