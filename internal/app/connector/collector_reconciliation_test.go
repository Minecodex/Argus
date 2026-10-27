package connector

import (
	"bytes"
	"context"
	"testing"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCollectorReconciliationReplaysOnlyRequestedPrivateJournalReceipts(t *testing.T) {
	store := localStore{directory: t.TempDir()}
	typed, _ := anypb.New(&connectorv1.CollectorManagementResult{CollectorId: "owned-collector", EffectiveRevision: 4, AppliedConfigSha256: "owned-hash", Status: "converged"})
	command := &connectorv1.ConnectorCommand{CommandId: "owned", CommandType: "collector_management"}
	if err := store.saveResult(outcomeRecord(command, commandOutcome{result: typed})); err != nil {
		t.Fatal(err)
	}
	private, _ := anypb.New(&connectorv1.HostRuntimeObservation{InventoryJson: []byte("private-probe-canary")})
	if err := store.saveResult(outcomeRecord(&connectorv1.ConnectorCommand{CommandId: "private", CommandType: "host_inventory_probe"}, commandOutcome{result: private})); err != nil {
		t.Fatal(err)
	}
	records, err := store.loadResults()
	if err != nil {
		t.Fatal(err)
	}
	request := &connectorv1.CommandReconcileRequest{ConnectionEpoch: 2, CommandIds: []string{"owned", "owned", "private", "missing"}}
	frames := collectorReconciliationFrames(request, 2, records)
	if len(frames) != 1 || frames[0].GetCommandResult().GetCommandId() != "owned" || frames[0].GetCommandResult().GetConnectionEpoch() != 2 {
		t.Fatal("reconciliation expanded its command/type scope")
	}
	raw, _ := proto.Marshal(frames[0])
	if bytes.Contains(raw, []byte("private-probe-canary")) {
		t.Fatal("unrelated private result escaped")
	}
	if len(collectorReconciliationFrames(request, 3, records)) != 0 {
		t.Fatal("stale request epoch accepted")
	}
	bad := records["owned"]
	bad.ResultHash = "corrupted"
	records["owned"] = bad
	if len(collectorReconciliationFrames(request, 2, records)) != 0 {
		t.Fatal("corrupt private journal was replayed")
	}
}

func TestCollectorCompletionPersistsWhenTransportHasAlreadyEnded(t *testing.T) {
	transport, cancel := context.WithCancel(t.Context())
	cancel()
	store := localStore{directory: t.TempDir()}
	client := connectorClient{store: store, executionContext: t.Context()}
	// An invalid command fails before external execution. Its real typed failure
	// must still become a durable receipt when no session loop can receive it.
	payload, _ := anypb.New(&connectorv1.CollectorManagementCommand{})
	command := &connectorv1.ConnectorCommand{CommandId: "ended-session", CommandType: "collector_management", TypedPayload: payload, ExpiresAt: timestamppb.New(time.Now().Add(time.Minute))}
	client.executeCommand(transport, command, nil, nil, make(chan completedCommand))
	records, err := store.loadResults()
	if err != nil {
		t.Fatal(err)
	}
	if record, ok := records[command.CommandId]; !ok || record.Status != "failed" || len(record.Result) == 0 {
		t.Fatal("completed outcome was lost with the transport")
	}
}
