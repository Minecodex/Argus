package action

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestCancelledInstallationsHaveCancelledTerminalOutcomes(t *testing.T) {
	status, code, terminal, err := reconciledHostOnboardingOutcome(context.Background(), nil,
		db.HostOnboardingOperation{Status: "cancelled", ErrorCode: pgtype.Text{String: hostonboarding.HostCancellationErrorCode, Valid: true}})
	if err != nil || !terminal || status != "cancelled" || code.String != hostonboarding.HostCancellationErrorCode {
		t.Fatalf("Host cancellation remained unresolved: %s %s %t %v", status, code.String, terminal, err)
	}
	status, code, terminal, err = reconciledConnectorInstallOutcome(context.Background(), nil,
		db.ConnectorInstallOperation{Status: "cancelled", ErrorCode: pgtype.Text{String: hostonboarding.BastionCancellationErrorCode, Valid: true}})
	if err != nil || !terminal || status != "cancelled" || code.String != hostonboarding.BastionCancellationErrorCode {
		t.Fatalf("Bastion cancellation became a failure: %s %s %t %v", status, code.String, terminal, err)
	}
}
