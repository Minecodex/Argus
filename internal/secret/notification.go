package secret

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Notification leases cannot be borrowed by connector or MCP recipients. A
// live, fenced delivery task and its current channel are the authority.
func validateNotificationLease(ctx context.Context, q *db.Queries, enterprise uuid.UUID, r LeaseRequest, c db.Credential) error {
	if r.Protocol != "notification" || r.TargetResourceType != "alert_channel" || r.RuntimeTaskID == uuid.Nil || r.RecipientID != r.RuntimeTaskID.String() {
		return ErrInvalidLease
	}
	id, err := uuid.Parse(r.OperationRef)
	if err != nil {
		return ErrInvalidLease
	}
	owned, err := q.AlertNotificationTaskOwned(ctx, db.AlertNotificationTaskOwnedParams{ID: r.RuntimeTaskID, LeaseOwner: optionalText(r.LeaseOwner), FenceToken: r.FenceToken, Column4: id.String(), EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}})
	if err != nil {
		return fmt.Errorf("notification lease task: %w", err)
	}
	if !owned {
		return ErrInvalidLease
	}
	delivery, err := q.GetAlertDelivery(ctx, db.GetAlertDeliveryParams{EnterpriseID: enterprise, ID: id})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("notification lease delivery: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) || delivery.Status != "sending" || delivery.ChannelID != r.TargetResourceID {
		return ErrInvalidLease
	}
	channel, err := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: enterprise, ID: r.TargetResourceID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("notification lease channel: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) || channel.Kind != "channel" || channel.Version != delivery.ChannelVersion {
		return ErrInvalidLease
	}
	var spec struct {
		Enabled      bool      `json:"enabled"`
		CredentialID uuid.UUID `json:"credential_id"`
	}
	if json.Unmarshal(channel.Spec, &spec) != nil || !spec.Enabled || spec.CredentialID != c.ID {
		return ErrInvalidLease
	}
	return nil
}
