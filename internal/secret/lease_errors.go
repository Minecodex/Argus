package secret

import (
	"errors"
	"github.com/jackc/pgx/v5"
)

// Missing/revoked material is a permanent authorization outcome. Database or
// cancellation failures must remain distinguishable for execution retries.
func leaseReadFailure(err, missing error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return missing
	}
	return err
}
