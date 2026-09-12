package connector

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestBastionRemovalPayloadPreservesWindowsRestoreSnapshot(t *testing.T) {
	plan := hostremoval.Plan{OperationID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, HostID: uuid.New(), ConnectorID: uuid.New(), RemovalGeneration: 3, TargetPlatform: "windows_amd64", Address: "10.0.0.1", Port: 22, Username: "Administrator", PinnedHostKey: "SHA256:test", ManagedChangeID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, ManagedChangeBefore: json.RawMessage(`{"f_deny_ts_connections":1}`), ManagedChangeApplied: json.RawMessage(`{"f_deny_ts_connections":0}`)}
	raw, _ := json.Marshal(plan)
	hash := sha256.Sum256(raw)
	command, err := typedCommand(db.ConnectorCommand{ID: uuid.New(), CommandID: "cmd_test", CommandType: "host_connector_removal", OperationRef: plan.OperationID.UUID.String(), Payload: raw, PayloadHash: hash[:], ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	var payload connectorv1.HostConnectorRemoval
	if err = command.TypedPayload.UnmarshalTo(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ManagedChangeId != plan.ManagedChangeID.UUID.String() || !bytes.Equal(payload.ManagedChangeBeforeJson, plan.ManagedChangeBefore) || !bytes.Equal(payload.ManagedChangeAppliedJson, plan.ManagedChangeApplied) {
		t.Fatal("Bastion dispatch lost frozen RDP snapshot")
	}
}
