package hostremoval

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestRemovalFenceDistinguishesReconnectFromReplacement(t *testing.T) {
	op := db.HostRemovalOperation{EnterpriseID: uuid.New(), ConnectorID: uuid.New(), HostID: uuid.New(), TargetType: TargetManagedHost, ConnectorVersion: 2, ConnectionEpoch: 3}
	connector := db.Connector{ID: op.ConnectorID, EnterpriseID: op.EnterpriseID, HostID: uuid.NullUUID{UUID: op.HostID, Valid: true}, Role: "host", Version: 4, ConnectionEpoch: 9, Status: "online"}
	if !removalConnectorMatches(op, connector) {
		t.Fatal("normal reconnect and certificate rotation must preserve committed removal")
	}
	for _, status := range []string{"revoked", "uninstalled"} {
		changed := connector
		changed.Status = status
		if removalConnectorMatches(op, changed) {
			t.Fatalf("accepted %s identity", status)
		}
	}
	changed := connector
	changed.ID = uuid.New()
	if removalConnectorMatches(op, changed) {
		t.Fatal("accepted replacement identity")
	}
	changed = connector
	changed.HostID.UUID = uuid.New()
	if removalConnectorMatches(op, changed) {
		t.Fatal("accepted different host binding")
	}
	changed = connector
	changed.ConnectionEpoch = 1
	if removalConnectorMatches(op, changed) {
		t.Fatal("accepted rolled-back epoch")
	}
}
