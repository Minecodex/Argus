package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrInventoryUnavailable = errors.New("HOST_INVENTORY_UNAVAILABLE")

// RefreshHostInventory is a read-only local observation sent through the
// authoritative typed command queue, bound to the current Host Connector.
func (service Service) RefreshHostInventory(ctx context.Context, subject Subject, enterpriseID, hostID uuid.UUID, key string) error {
	host, err := service.GetHost(ctx, enterpriseID, hostID, subject.AuthorizedResourceIDs)
	if err != nil {
		return err
	}
	if host.Status != "active" || !host.ConnectorID.Valid || service.Commands == nil {
		return ErrInventoryUnavailable
	}
	connector, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: host.ConnectorID.UUID, EnterpriseID: enterpriseID})
	if err != nil || connector.Status != "online" || connector.ConnectionEpoch < 1 || !connector.HostID.Valid || connector.HostID.UUID != hostID || (connector.Role != "host" && connector.Role != "bastion") {
		return ErrInventoryUnavailable
	}
	digest := sha256.Sum256([]byte(subject.ActorID + ":" + hostID.String() + ":" + key))
	operation := "host_inventory:" + hex.EncodeToString(digest[:])
	payload := []byte(`{}`)
	hash := sha256.Sum256(payload)
	command, err := service.Store.Queries.GetLatestConnectorCommandForOperation(ctx, db.GetLatestConnectorCommandForOperationParams{EnterpriseID: enterpriseID, OperationRef: operation})
	if errors.Is(err, pgx.ErrNoRows) {
		err = service.Store.InTx(ctx, func(q *db.Queries) error {
			id := uuid.New()
			var createErr error
			command, createErr = q.CreateConnectorCommand(ctx, db.CreateConnectorCommandParams{ID: id, CommandID: id.String(), EnterpriseID: enterpriseID, ConnectorID: connector.ID, ConnectionEpoch: connector.ConnectionEpoch, OperationRef: operation, CommandType: "host_inventory_probe", PayloadSchemaVersion: "argus.connector.command/v1", Payload: payload, PayloadHash: hash[:], IdempotencyKey: operation, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}})
			if createErr != nil {
				return createErr
			}
			return appendResourceAudit(ctx, q, subject.ActorID, enterpriseID, "host.inventory.request", "host", hostID, map[string]any{"command_id": command.CommandID, "connector_id": connector.ID})
		})
		if err != nil {
			command, err = service.Store.Queries.GetLatestConnectorCommandForOperation(ctx, db.GetLatestConnectorCommandForOperationParams{EnterpriseID: enterpriseID, OperationRef: operation})
		}

	}
	if err != nil {
		return err
	}
	if command.ConnectorID != connector.ID || command.ConnectionEpoch != connector.ConnectionEpoch {
		return ErrInventoryUnavailable
	}
	service.Commands.NotifyConnectorCommand(ctx, connector.ID, connector.ConnectionEpoch)
	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if command.Status == "succeeded" {
			return nil
		}
		if command.Status == "failed" || command.Status == "expired" || command.Status == "result_unknown" || command.Status == "delivery_unknown" {
			return ErrInventoryUnavailable
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return ErrInventoryUnavailable
		case <-ticker.C:
		}
		if _, err = service.GetHost(ctx, enterpriseID, hostID, subject.AuthorizedResourceIDs); err != nil {
			return err
		}
		command, err = service.Store.Queries.GetConnectorCommandByID(ctx, command.ID)
		if err != nil {
			return err
		}
	}
}
