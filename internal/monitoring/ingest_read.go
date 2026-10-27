package monitoring

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type IngestEndpoint struct {
	IngestEndpointInput
	ID             uuid.UUID  `json:"id"`
	Version        int64      `json:"version"`
	LastReceivedAt *time.Time `json:"last_received_at,omitempty"`
	LastRejectedAt *time.Time `json:"last_rejected_at,omitempty"`
	LastErrorCode  string     `json:"last_error_code"`
}
type IngestKey struct {
	ID         uuid.UUID  `json:"id"`
	EndpointID uuid.UUID  `json:"endpoint_id"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Revealed   bool       `json:"revealed"`
	CreatedAt  time.Time  `json:"created_at"`
	Version    int64      `json:"version"`
}

func ingestTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func (s Service) IngestEndpoints(ctx context.Context, a Actor, application uuid.UUID, in ListInput) (Page[IngestEndpoint], error) {
	out := Page[IngestEndpoint]{Items: []IngestEndpoint{}}
	if _, err := s.Application(ctx, a, application); err != nil {
		return out, err
	}
	in.ApplicationIDs = []uuid.UUID{application}
	limit, after, binding, err := s.page(a, "ingest_endpoints", in)
	if err != nil {
		return out, err
	}
	rows, err := s.Store.Queries.ListMonitoringIngestEndpoints(ctx, db.ListMonitoringIngestEndpointsParams{EnterpriseID: a.EnterpriseID, ApplicationID: application, AfterID: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor, err = s.next(binding, rows[limit-1].ID)
		if err != nil {
			return out, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, IngestEndpoint{IngestEndpointInput: IngestEndpointInput{Name: row.Name, ApplicationID: row.ApplicationID, CollectorID: row.CollectorID.UUID, Protocol: row.Protocol, Signals: row.Signals, ServiceAllowlist: row.ServiceAllowlist, Enabled: row.Enabled}, ID: row.ID, Version: row.Version, LastReceivedAt: ingestTime(row.LastReceivedAt), LastRejectedAt: ingestTime(row.LastRejectedAt), LastErrorCode: row.LastErrorCode})
	}
	return out, authorization.Authorize(ctx, s.Store.Queries, a, "application.read")
}
func (s Service) IngestKeys(ctx context.Context, a Actor, endpoint uuid.UUID, in ListInput) (Page[IngestKey], error) {
	out := Page[IngestKey]{Items: []IngestKey{}}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "application.read"); err != nil {
		return out, err
	}
	row, err := s.Store.Queries.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: endpoint})
	if err != nil {
		return out, lookupError(err)
	}
	if _, err = s.Application(ctx, a, row.ApplicationID); err != nil {
		return out, err
	}
	limit, after, binding, err := s.page(a, "ingest_keys:"+endpoint.String(), in)
	if err != nil {
		return out, err
	}
	keys, err := s.Store.Queries.ListMonitoringIngestKeys(ctx, db.ListMonitoringIngestKeysParams{EnterpriseID: a.EnterpriseID, EndpointID: endpoint, AfterID: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(keys) > limit {
		out.NextCursor, err = s.next(binding, keys[limit-1].ID)
		if err != nil {
			return out, err
		}
		keys = keys[:limit]
	}
	for _, key := range keys {
		out.Items = append(out.Items, IngestKey{ID: key.ID, EndpointID: key.EndpointID, Prefix: key.KeyPrefix, ExpiresAt: ingestTime(key.ExpiresAt), RevokedAt: ingestTime(key.RevokedAt), LastUsedAt: ingestTime(key.LastUsedAt), Revealed: key.RevealedAt.Valid, CreatedAt: key.CreatedAt.Time, Version: key.Version})
	}
	return out, authorization.Authorize(ctx, s.Store.Queries, a, "application.read")
}
