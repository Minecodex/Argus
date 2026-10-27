package monitoring

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// The one-time reveal value is derived with a separate domain from the action
// signing secret. PostgreSQL holds only its digest; no pending action, audit,
// idempotency response or model tool receives the bearer value.
func (s Service) deriveIngestKey(enterprise, id uuid.UUID) string {
	mac := hmac.New(sha256.New, s.Actions.Key)
	mac.Write([]byte("argus.application_ingest.key/v1\x00"))
	mac.Write(enterprise[:])
	mac.Write(id[:])
	return "argus_app." + id.String() + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func IngestKeyID(value string) (uuid.UUID, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "argus_app" || len(parts[2]) != 43 {
		return uuid.Nil, ErrIngestRejected
	}
	id, err := uuid.Parse(parts[1])
	secret, e := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || id == uuid.Nil || e != nil || len(secret) != 32 {
		return uuid.Nil, ErrIngestRejected
	}
	return id, nil
}

// SkyWalking's locked native receiver acknowledges consumer failures and its
// synchronous RPC prints raw segments. Keep it catalogued, but fail admission
// until a qualifying upstream implementation passes the same gates.
func SDKProtocolEligible(protocol string) bool { return protocol == "otlp" || protocol == "jaeger" }

func AuthenticateIngestKey(ctx context.Context, q *db.Queries, collector, enterprise uuid.UUID, protocol, signal, token string) (db.ResolveMonitoringIngestKeyRow, error) {
	var empty db.ResolveMonitoringIngestKeyRow
	if q == nil || collector == uuid.Nil || enterprise == uuid.Nil || !SDKProtocolEligible(protocol) {
		return empty, ErrIngestRejected
	}
	id, err := IngestKeyID(token)
	if err != nil {
		return empty, err
	}
	row, err := q.ResolveMonitoringIngestKey(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return empty, ErrIngestRejected
		}
		return empty, err
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(digest[:], row.KeyDigest) != 1 || row.EnterpriseID != enterprise || !row.CollectorID.Valid || row.CollectorID.UUID != collector || row.Protocol != protocol {
		return empty, ErrIngestRejected
	}
	if !sdkDistributionEligible(protocol, row.DistributionVersion, row.CollectorVersion, row.ConfigSchemaVersion, row.SupportStatus, row.Components) {
		return empty, ErrIngestRejected
	}
	allowed := signal == ""
	for _, value := range row.Signals {
		if value == signal {
			allowed = true
		}
	}
	if !allowed {
		return empty, ErrIngestRejected
	}
	return row, nil
}

// RevealIngestKey is a dedicated interactive portal operation. It is purposely
// absent from the Agent tool surface and rejects service accounts and runs.
func (s Service) RevealIngestKey(ctx context.Context, a Actor, id uuid.UUID) (string, error) {
	if s.Store == nil || len(s.Actions.Key) < 32 {
		return "", ErrUnavailable
	}
	if a.SubjectType != "user" || a.RunID != uuid.Nil {
		return "", ErrDenied
	}
	var result string
	err := s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		row, err := q.GetMonitoringIngestKey(ctx, db.GetMonitoringIngestKeyParams{EnterpriseID: a.EnterpriseID, ID: id})
		if err != nil {
			return lookupError(err)
		}
		if err = s.collectionApplication(ctx, q, a, row.ApplicationID, uuid.Nil, map[string]int64{}); err != nil {
			return err
		}
		value := s.deriveIngestKey(a.EnterpriseID, id)
		digest := sha256.Sum256([]byte(value))
		if subtle.ConstantTimeCompare(digest[:], row.KeyDigest) != 1 {
			return ErrConflict
		}
		if _, err = q.ConsumeMonitoringKeyReveal(ctx, db.ConsumeMonitoringKeyRevealParams{EnterpriseID: a.EnterpriseID, ID: id, CreatedBy: a.SubjectID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			return err
		}
		if _, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: nullID(a.EnterpriseID), ActorType: "enterprise_user", ActorID: a.SubjectID.String(), Action: "application.ingest_key.reveal", ResourceType: "ingest_key", ResourceID: id.String(), Result: "success", Details: map[string]any{"application_id": row.ApplicationID.String()}}); err != nil {
			return err
		}
		result = value
		return nil
	})
	if err != nil {
		return "", err
	}
	return result, nil
}
