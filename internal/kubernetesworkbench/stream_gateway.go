package kubernetesworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) ClaimStream(ctx context.Context, id uuid.UUID, ticket, owner string) (StreamConnection, error) {
	value, err := base64.RawURLEncoding.DecodeString(ticket)
	if err != nil || len(value) != 32 || owner == "" || len(owner) > 200 {
		return StreamConnection{}, authorization.ErrAccessDenied
	}
	defer clear(value)
	hash := sha256.Sum256(value)
	var result StreamConnection
	err = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		row, err := q.GetKubernetesStreamSession(ctx, id)
		if err != nil {
			return authorization.ErrAccessDenied
		}
		result, err = authorizeStreamRow(ctx, q, row)
		if err != nil {
			return err
		}
		row, err = q.ClaimKubernetesStreamSession(ctx, db.ClaimKubernetesStreamSessionParams{ID: id, TicketHash: hash[:], OwnerGateway: owner})
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.ErrAccessDenied
		}
		if err != nil {
			return err
		}
		result.Row = row
		if result.Binding.Route.CredentialID != uuid.Nil {
			recipient, recipientID := "connector", result.Binding.Route.ConnectorID.String()
			if result.Binding.Route.Mode == "direct" {
				recipient, recipientID = "connector_gateway", owner
			}
			lease, err := s.Credentials.PrepareLeaseWithQueries(secret.WithActorType(ctx, "system"), q, owner, row.EnterpriseID, secret.LeaseRequest{CredentialID: result.Binding.Route.CredentialID, OperationRef: id.String(), TargetResourceType: "kubernetes_stream_session", TargetResourceID: id, RecipientType: recipient, RecipientID: recipientID, Protocol: "kubernetes", TTL: 5 * time.Minute})
			if err != nil {
				return err
			}
			if lease.SecretVersionID != result.Binding.Route.SecretVersionID {
				return kubernetesinventory.ErrIdentityChanged
			}
			result.Row, err = q.BindKubernetesStreamLease(ctx, db.BindKubernetesStreamLeaseParams{ID: id, Fence: row.Fence, OwnerGateway: owner, CredentialLeaseID: uuid.NullUUID{UUID: lease.ID, Valid: true}, ExpiresAt: lease.ExpiresAt})
			if err != nil {
				return err
			}
		}
		return streamAudit(ctx, q, result.Row, "connecting")
	})
	return result, err
}
func (s Service) CurrentStream(ctx context.Context, id uuid.UUID, fence int64) (StreamConnection, error) {
	row, err := s.Store.Queries.GetKubernetesStreamSession(ctx, id)
	if err != nil || row.Fence != fence || row.Status != "connecting" && row.Status != "active" {
		return StreamConnection{}, authorization.ErrAccessDenied
	}
	return authorizeStreamRow(ctx, s.Store.Queries, row)
}
func (s Service) ActivateStream(ctx context.Context, id uuid.UUID, fence int64, owner string) error {
	current, err := s.CurrentStream(ctx, id, fence)
	if err != nil {
		return err
	}
	if current.Row.ExecutionGateway == "" {
		return kubernetesinventory.ErrMutationConflict
	}
	row, err := s.Store.Queries.ActivateKubernetesStreamSession(ctx, db.ActivateKubernetesStreamSessionParams{ID: id, Fence: fence, OwnerGateway: owner})
	if err != nil {
		return err
	}
	return streamAudit(ctx, s.Store.Queries, row, "active")
}

func (s Service) ClaimStreamExecution(ctx context.Context, id uuid.UUID, fence int64, executor string) error {
	if executor == "" || len(executor) > 200 {
		return kubernetesinventory.ErrInvalid
	}
	if _, err := s.CurrentStream(ctx, id, fence); err != nil {
		return err
	}
	n, err := s.Store.Queries.ClaimKubernetesStreamExecution(ctx, db.ClaimKubernetesStreamExecutionParams{ID: id, Fence: fence, ExecutionGateway: executor})
	if err != nil {
		return err
	}
	if n != 1 {
		return kubernetesinventory.ErrMutationConflict
	}
	return nil
}
func (s Service) HeartbeatStream(ctx context.Context, id uuid.UUID, fence int64, owner string) error {
	if _, err := s.CurrentStream(ctx, id, fence); err != nil {
		return err
	}
	count, err := s.Store.Queries.HeartbeatKubernetesStreamSession(ctx, db.HeartbeatKubernetesStreamSessionParams{ID: id, Fence: fence, OwnerGateway: owner})
	if err != nil {
		return err
	}
	if count != 1 {
		return authorization.ErrAccessDenied
	}
	return nil
}
