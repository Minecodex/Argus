package connector

import (
	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Deleting one pending member must not tear down its shared Bastion stream
// when an acknowledgement or result that was already in flight arrives.
func cancelledHostInstallCommand(command db.ConnectorCommand) bool {
	return command.CommandType == "host_connector_install" && command.Status == "expired" &&
		command.ErrorCode.Valid && command.ErrorCode.String == hostonboarding.HostCancellationErrorCode
}
