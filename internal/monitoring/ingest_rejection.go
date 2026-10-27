package monitoring

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// ObserveRejectedIngestKey records a diagnostic only after verifying possession
// of the original secret, including revoked/expired keys. A guessed Key ID or
// an incorrect secret cannot poison another application's ingestion state.
// The caller must already have authenticated the Collector through mTLS.
func (s Service) ObserveRejectedIngestKey(ctx context.Context, enterprise, collector uuid.UUID, protocol, token string) error {
	if s.Store == nil || enterprise == uuid.Nil || collector == uuid.Nil {
		return ErrUnavailable
	}
	id, err := IngestKeyID(token)
	if err != nil || !SDKProtocolEligible(protocol) {
		return nil
	}
	q := s.Store.Queries
	key, err := q.GetMonitoringIngestKey(ctx, db.GetMonitoringIngestKeyParams{EnterpriseID: enterprise, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(digest[:], key.KeyDigest) != 1 {
		return nil
	}
	endpoint, err := q.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: enterprise, ID: key.EndpointID})
	if err != nil {
		return err
	}
	if endpoint.ApplicationID != key.ApplicationID || !endpoint.CollectorID.Valid || endpoint.CollectorID.UUID != collector || endpoint.Protocol != protocol {
		return nil
	}
	code := "APPLICATION_INGEST_REJECTED"
	switch {
	case key.RevokedAt.Valid:
		code = "APPLICATION_INGEST_KEY_REVOKED"
	case key.ExpiresAt.Valid && !s.now().Before(key.ExpiresAt.Time):
		code = "APPLICATION_INGEST_KEY_EXPIRED"
	case !endpoint.Enabled:
		code = "APPLICATION_INGEST_ENDPOINT_DISABLED"
	}
	return q.RejectApplicationIngest(ctx, db.RejectApplicationIngestParams{EnterpriseID: enterprise, ID: endpoint.ID, LastErrorCode: code})
}
