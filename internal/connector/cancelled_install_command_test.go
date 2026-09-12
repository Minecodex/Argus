package connector

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestCancelledInstallCommandOnlyDiscardsItsOwnLateReplies(t *testing.T) {
	for _, test := range []struct {
		kind, status, code string
		want               bool
	}{
		{"host_connector_install", "expired", hostonboarding.HostCancellationErrorCode, true},
		{"host_connector_install", "running", hostonboarding.HostCancellationErrorCode, false},
		{"host_connector_install", "expired", "CONNECTOR_COMMAND_EXPIRED", false},
		{"host_connector_removal", "expired", hostonboarding.HostCancellationErrorCode, false},
		{"collector_management", "expired", hostonboarding.HostCancellationErrorCode, false},
	} {
		command := db.ConnectorCommand{CommandType: test.kind, Status: test.status, ErrorCode: pgtype.Text{String: test.code, Valid: true}}
		if cancelledHostInstallCommand(command) != test.want {
			t.Fatalf("unexpected cancellation handling for %s/%s/%s", test.kind, test.status, test.code)
		}
	}
}
