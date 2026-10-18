package connector

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Reserve transport capacity for explicit stop, even when long batches occupy
// the ordinary command window. This does not create a resource-level queue.
func isRemoteStopControl(command db.ConnectorCommand) bool {
	if command.CommandType != "remote_execution_control" {
		return false
	}
	var control struct {
		Operation string `json:"operation"`
	}
	return json.Unmarshal(command.Payload, &control) == nil && control.Operation == "stop"
}
