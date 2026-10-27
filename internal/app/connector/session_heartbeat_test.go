package connector

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/grpc"
)

type firstHeartbeatStream struct {
	grpc.ClientStream
	ctx  context.Context
	sent chan *connectorv1.ConnectRequest
	err  error
}

func (s *firstHeartbeatStream) Send(value *connectorv1.ConnectRequest) error {
	if s.err != nil {
		return s.err
	}
	select {
	case s.sent <- value:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *firstHeartbeatStream) Recv() (*connectorv1.ConnectResponse, error) {
	<-s.ctx.Done()
	return nil, io.EOF
}

func TestSessionImmediatelyReportsRelayHealthForEveryConnectionEpoch(t *testing.T) {
	for _, epoch := range []uint64{1, 9} {
		ctx, cancel := context.WithCancel(t.Context())
		stream := &firstHeartbeatStream{ctx: ctx, sent: make(chan *connectorv1.ConnectRequest, 1)}
		relay := &bastionRelay{generation: 3, advertiseAddress: "192.0.2.10", reported: []*connectorv1.BastionRelayListener{
			{Kind: "https", Port: 8445, Status: "ready"}, {Kind: "gateway", Port: 9445, Status: "ready"},
		}}
		relay.ready.Store(true)
		client := connectorClient{store: localStore{directory: t.TempDir()}, relay: relay}
		tunnels := newMemberTunnelSupervisor(connectorTelemetryTunnelLimit, connectorTunnelBytesPerSecond())
		started := time.Now()
		done := make(chan error, 1)
		go func() {
			done <- client.sessionLoop(ctx, stream, identityState{Role: "bastion", TrustBundleEpoch: 4},
				&connectorv1.ConnectorWelcome{ConnectionEpoch: epoch}, time.Minute, tunnels, func(error) { cancel() })
		}()
		select {
		case frame := <-stream.sent:
			heartbeat := frame.GetHeartbeat()
			if frame.Sequence != 2 || heartbeat == nil || heartbeat.ConnectionEpoch != epoch || heartbeat.GetTrustBundleEpoch() != 4 ||
				heartbeat.GetBastionRelay().GetStatus() != "ready" || heartbeat.GetBastionRelay().GetGeneration() != 3 ||
				heartbeat.GetHostRuntime() == nil || heartbeat.GetHostRuntime().GetObservedAt().AsTime().Before(started) {
				t.Error("first frame did not contain the current epoch and freshly observed relay/host health")
			}
		case err := <-done:
			t.Errorf("session exited before its first heartbeat: %v", err)
		case <-time.After(15 * time.Second):
			t.Error("session waited for the periodic heartbeat instead of reporting health on connection")
		}
		cancel()
		tunnels.CloseAll()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("cancelled session did not stop")
		}
	}
}

func TestSessionDoesNotIgnoreInitialHeartbeatWriteFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	want := errors.New("stream write failed")
	stream := &firstHeartbeatStream{ctx: ctx, err: want}
	client := connectorClient{store: localStore{directory: t.TempDir()}}
	tunnels := newMemberTunnelSupervisor(connectorTelemetryTunnelLimit, connectorTunnelBytesPerSecond())
	defer tunnels.CloseAll()
	if err := client.sessionLoop(ctx, stream, identityState{}, &connectorv1.ConnectorWelcome{ConnectionEpoch: 2}, time.Minute, tunnels, func(error) { cancel() }); !errors.Is(err, want) {
		t.Fatalf("initial heartbeat failure = %v", err)
	}
}
