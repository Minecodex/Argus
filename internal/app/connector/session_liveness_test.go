package connector

import (
	"context"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type p8SilentGateway struct {
	connectorv1.UnimplementedConnectorControlServiceServer
	heartbeats atomic.Int32
}

func (s *p8SilentGateway) Connect(stream connectorv1.ConnectorControlService_ConnectServer) error {
	for {
		frame, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if frame.GetHeartbeat() != nil {
			s.heartbeats.Add(1)
		}
		// The TCP/HTTP2 transport still accepts every heartbeat. The peer never
		// acknowledges them, as with a silent forwarding/backend failure.
	}
}

func TestControlSessionTimesOutForSilentGateway(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	peer := &p8SilentGateway{}
	connectorv1.RegisterConnectorControlServiceServer(server, peer)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	defer listener.Close()
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	connection, err := grpc.NewClient("passthrough:///owned-silent-gateway", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	stream, err := connectorv1.NewConnectorControlServiceClient(connection).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := connectorClient{store: localStore{directory: t.TempDir()}}
	tunnels := newMemberTunnelSupervisor(connectorTelemetryTunnelLimit, connectorTunnelBytesPerSecond())
	defer tunnels.CloseAll()
	done := make(chan error, 1)
	go func() {
		done <- client.sessionLoop(ctx, stream, identityState{}, &connectorv1.ConnectorWelcome{ConnectionEpoch: 2}, 50*time.Millisecond, tunnels, cancel)
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
			t.Fatalf("silent gateway ended with %v instead of a liveness timeout", err)
		}
	case <-time.After(time.Second):
		cancel(nil)
		<-done
		t.Fatalf("control session stayed open without any server acknowledgement; peer received %d actual gRPC heartbeats", peer.heartbeats.Load())
	}
	if peer.heartbeats.Load() < 1 {
		t.Fatal("failure occurred before an actual heartbeat reached the gRPC peer")
	}
}

type p8AcknowledgingGateway struct {
	connectorv1.UnimplementedConnectorControlServiceServer
	progress bool
}

func (s *p8AcknowledgingGateway) Connect(stream connectorv1.ConnectorControlService_ConnectServer) error {
	sequence, ack := uint64(2), uint64(0)
	for {
		frame, err := stream.Recv()
		if err != nil {
			return err
		}
		if s.progress || ack == 0 {
			ack = frame.GetSequence()
		}
		if err = stream.Send(&connectorv1.ConnectResponse{Sequence: sequence, Frame: &connectorv1.ConnectResponse_Acknowledge{Acknowledge: &connectorv1.ServerAcknowledge{ClientSequence: ack}}}); err != nil {
			return err
		}
		sequence++
	}
}
func TestControlSessionRequiresAdvancingAcknowledgements(t *testing.T) {
	for _, progress := range []bool{false, true} {
		name := "stale"
		if progress {
			name = "advancing"
		}
		t.Run(name, func(t *testing.T) {
			listener := bufconn.Listen(1 << 20)
			server := grpc.NewServer()
			connectorv1.RegisterConnectorControlServiceServer(server, &p8AcknowledgingGateway{progress: progress})
			go func() { _ = server.Serve(listener) }()
			defer server.Stop()
			defer listener.Close()
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			connection, err := grpc.NewClient("passthrough:///owned-acknowledging-gateway", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			stream, err := connectorv1.NewConnectorControlServiceClient(connection).Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			client := connectorClient{store: localStore{directory: t.TempDir()}}
			tunnels := newMemberTunnelSupervisor(connectorTelemetryTunnelLimit, connectorTunnelBytesPerSecond())
			defer tunnels.CloseAll()
			done := make(chan error, 1)
			go func() {
				done <- client.sessionLoop(ctx, stream, identityState{}, &connectorv1.ConnectorWelcome{ConnectionEpoch: 2}, 50*time.Millisecond, tunnels, cancel)
			}()
			if progress {
				select {
				case err := <-done:
					t.Fatalf("healthy acknowledgements did not preserve the control session: %v", err)
				case <-time.After(500 * time.Millisecond):
				}
				cancel(nil)
				<-done
			} else {
				select {
				case err := <-done:
					if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
						t.Fatalf("stale acknowledgements ended with %v", err)
					}
				case <-time.After(time.Second):
					cancel(nil)
					<-done
					t.Fatal("repeated old acknowledgements kept the session alive")
				}
			}
		})
	}
}
