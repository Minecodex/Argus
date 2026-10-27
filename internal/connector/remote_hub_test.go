package connector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteaccess"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type hubOpenResult struct {
	session remoteaccess.BackendSession
	err     error
}

func startHubOpen(hub *RemoteAccessHub, connectorID uuid.UUID, epoch int64) <-chan hubOpenResult {
	done := make(chan hubOpenResult, 1)
	go func() {
		session, err := hub.Open(context.Background(), remoteaccess.ConnectionTarget{
			Session:         db.RemoteAccessSession{ID: uuid.New(), SessionFence: 1},
			ConnectorID:     uuid.NullUUID{UUID: connectorID, Valid: true},
			ConnectionEpoch: epoch,
		}, 100, 30)
		done <- hubOpenResult{session: session, err: err}
	}()
	return done
}

func readRemoteAccessOpen(t *testing.T, outbound <-chan *connectorv1.ConnectResponse) *connectorv1.RemoteAccessOpen {
	t.Helper()
	select {
	case frame := <-outbound:
		open := frame.GetRemoteAccessOpen()
		if open == nil {
			t.Fatalf("expected RemoteAccessOpen frame, got %T", frame.GetFrame())
		}
		return open
	case <-time.After(time.Second):
		t.Fatal("hub did not enqueue RemoteAccessOpen frame")
		return nil
	}
}

func deliverRemoteState(t *testing.T, hub *RemoteAccessHub, connectorID uuid.UUID, epoch int64, streamID string, sequence uint64, state connectorv1.RemoteAccessStateValue) {
	t.Helper()
	request := &connectorv1.ConnectRequest{Frame: &connectorv1.ConnectRequest_RemoteAccessState{RemoteAccessState: &connectorv1.RemoteAccessState{
		StreamId: streamID, StreamSequence: sequence, State: state}}}
	if err := hub.Deliver(connectorID, epoch, request); err != nil {
		t.Fatalf("Deliver rejected %v frame: %v", state, err)
	}
}

func TestRemoteAccessHubOpenWaitsForActiveState(t *testing.T) {
	hub := NewRemoteAccessHub()
	hub.HandshakeTimeout = 2 * time.Second
	connectorID := uuid.New()
	outbound := make(chan *connectorv1.ConnectResponse, 32)
	unregister := hub.Register(connectorID, 1, outbound)
	defer unregister()

	done := startHubOpen(hub, connectorID, 1)
	open := readRemoteAccessOpen(t, outbound)
	deliverRemoteState(t, hub, connectorID, 1, open.GetStreamId(), 1,
		connectorv1.RemoteAccessStateValue_REMOTE_ACCESS_STATE_VALUE_ACTIVE)

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("Open failed after active state: %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open did not return after active state")
	}
}

func TestRemoteAccessHubOpenFailsOnTerminalState(t *testing.T) {
	hub := NewRemoteAccessHub()
	hub.HandshakeTimeout = 2 * time.Second
	connectorID := uuid.New()
	outbound := make(chan *connectorv1.ConnectResponse, 32)
	unregister := hub.Register(connectorID, 1, outbound)
	defer unregister()

	done := startHubOpen(hub, connectorID, 1)
	open := readRemoteAccessOpen(t, outbound)
	deliverRemoteState(t, hub, connectorID, 1, open.GetStreamId(), 1,
		connectorv1.RemoteAccessStateValue_REMOTE_ACCESS_STATE_VALUE_CONNECTION_LOST)

	select {
	case result := <-done:
		if !errors.Is(result.err, remoteaccess.ErrSessionUnavailable) {
			t.Fatalf("expected ErrSessionUnavailable, got %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open did not return after terminal state")
	}
	// The failed stream must be gone so late frames are dropped, not delivered.
	if err := hub.Deliver(connectorID, 1, &connectorv1.ConnectRequest{Frame: &connectorv1.ConnectRequest_RemoteAccessOutput{
		RemoteAccessOutput: &connectorv1.RemoteAccessOutput{StreamId: open.GetStreamId(), StreamSequence: 2,
			Stream: connectorv1.RemoteAccessOutputStream_REMOTE_ACCESS_OUTPUT_STREAM_STDOUT}}}); err != nil {
		t.Fatalf("late frame for removed stream must be dropped, got %v", err)
	}
}

func TestRemoteAccessHubOpenTimesOutWithoutState(t *testing.T) {
	hub := NewRemoteAccessHub()
	hub.HandshakeTimeout = 50 * time.Millisecond
	connectorID := uuid.New()
	outbound := make(chan *connectorv1.ConnectResponse, 32)
	unregister := hub.Register(connectorID, 1, outbound)
	defer unregister()

	done := startHubOpen(hub, connectorID, 1)
	readRemoteAccessOpen(t, outbound)

	select {
	case result := <-done:
		if !errors.Is(result.err, remoteaccess.ErrSessionUnavailable) {
			t.Fatalf("expected ErrSessionUnavailable on timeout, got %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Open did not time out")
	}
	// Timeout cleanup must ask the Connector to stop the remote session.
	select {
	case frame := <-outbound:
		if frame.GetRemoteAccessClose() == nil {
			t.Fatalf("expected RemoteAccessClose after handshake timeout, got %T", frame.GetFrame())
		}
	case <-time.After(time.Second):
		t.Fatal("handshake timeout did not send RemoteAccessClose to Connector")
	}
}

func TestRemoteAccessHubOpenFailsWhenConnectorDisconnects(t *testing.T) {
	hub := NewRemoteAccessHub()
	hub.HandshakeTimeout = 2 * time.Second
	connectorID := uuid.New()
	outbound := make(chan *connectorv1.ConnectResponse, 32)
	unregister := hub.Register(connectorID, 1, outbound)

	done := startHubOpen(hub, connectorID, 1)
	readRemoteAccessOpen(t, outbound)
	unregister()

	select {
	case result := <-done:
		if !errors.Is(result.err, remoteaccess.ErrSessionUnavailable) {
			t.Fatalf("expected ErrSessionUnavailable on Connector disconnect, got %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open did not return after Connector disconnect")
	}
}

func TestRemoteAccessHubOpenValidatesBinding(t *testing.T) {
	hub := NewRemoteAccessHub()
	connectorID := uuid.New()
	outbound := make(chan *connectorv1.ConnectResponse, 32)
	unregister := hub.Register(connectorID, 3, outbound)
	defer unregister()

	if _, err := hub.Open(context.Background(), remoteaccess.ConnectionTarget{
		ConnectorID:     uuid.NullUUID{UUID: connectorID, Valid: true},
		ConnectionEpoch: 2,
	}, 100, 30); !errors.Is(err, remoteaccess.ErrSessionUnavailable) {
		t.Fatalf("stale epoch must be rejected, got %v", err)
	}
}

func TestKubernetesStreamsRequireAdvertisedConnectorCapability(t *testing.T) {
	for _, role := range []string{"bastion", "kubernetes"} {
		if !capabilitiesAllowed(role, []string{"kubernetes.query", "argus.kubernetes_stream/v1"}) {
			t.Fatal("native handshake capability rejected", role)
		}
	}
	if capabilitiesAllowed("host", []string{"argus.kubernetes_stream/v1"}) {
		t.Fatal("host Connector acquired Kubernetes capability")
	}
	hub := NewRemoteAccessHub()
	id := uuid.New()
	outbound := make(chan *connectorv1.ConnectResponse, 8)
	unregister := hub.Register(id, 1, outbound)
	request := &connectorv1.RemoteAccessOpen{SessionId: uuid.NewString(), SessionFence: 1, Protocol: "kubernetes_exec", KubernetesTargetJson: []byte(`{"schema_version":"argus.kubernetes_stream/v1"}`)}
	if _, err := hub.OpenPrepared(t.Context(), id, 1, request); !errors.Is(err, remoteaccess.ErrProtocol) {
		t.Fatal("legacy Connector received unsupported protocol", err)
	}
	if len(outbound) != 0 {
		t.Fatal("unsupported frame was enqueued")
	}
	unregister()
	unregister = hub.Register(id, 2, outbound, "argus.kubernetes_stream/v1")
	defer unregister()
	done := make(chan hubOpenResult, 1)
	go func() { s, e := hub.OpenPrepared(t.Context(), id, 2, request); done <- hubOpenResult{s, e} }()
	open := readRemoteAccessOpen(t, outbound)
	if open.Protocol != "kubernetes_exec" || open.ConnectionEpoch != 2 || string(open.KubernetesTargetJson) != string(request.KubernetesTargetJson) {
		t.Fatal("native binding changed")
	}
	deliverRemoteState(t, hub, id, 2, open.StreamId, 1, connectorv1.RemoteAccessStateValue_REMOTE_ACCESS_STATE_VALUE_ACTIVE)
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	_ = result.session.Close(context.Background(), "test_cleanup")
}
