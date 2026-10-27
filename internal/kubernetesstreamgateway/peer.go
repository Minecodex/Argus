package kubernetesstreamgateway

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	remotev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/remoteaccess/v1"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/remoteaccess"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PeerDialer struct {
	Transport *remoteaccess.GatewayPeerDialer
}

func (p PeerDialer) Open(ctx context.Context, owner string, v kubernetesworkbench.StreamConnection, cols, rows int) (interactive.Session, error) {
	d := p.Transport
	if d == nil || d.TLSCredentials == nil || owner == "" {
		return nil, remoteaccess.ErrSessionUnavailable
	}
	endpoint := fmt.Sprintf("%s.%s:%s", owner, d.HeadlessSuffix, d.Port)
	if d.Resolver != nil {
		var e error
		endpoint, e = d.Resolver.ResolveEndpoint(ctx, owner, d.Port)
		if e != nil {
			return nil, e
		}
	}
	connection, e := grpc.NewClient(endpoint, grpc.WithTransportCredentials(d.TLSCredentials.Clone()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<10), grpc.MaxCallSendMsgSize(64<<10)))
	if e != nil {
		return nil, e
	}
	deadline := time.AfterFunc(20*time.Second, func() { _ = connection.Close() })
	defer deadline.Stop()
	stream, e := remotev1.NewKubernetesStreamPeerServiceClient(connection).OpenKubernetesStream(ctx)
	if e != nil {
		_ = connection.Close()
		return nil, e
	}
	lease := ""
	if v.Row.CredentialLeaseID.Valid {
		lease = v.Row.CredentialLeaseID.UUID.String()
	}
	e = stream.Send(&remotev1.OpenKubernetesStreamRequest{Sequence: 1, Frame: &remotev1.OpenKubernetesStreamRequest_Open{Open: &remotev1.GatewayPeerOpen{SessionId: v.Row.ID.String(), SessionFence: uint64(v.Row.Fence), ConnectorId: v.Binding.Route.ConnectorID.String(), ConnectionEpoch: uint64(v.Binding.Route.Epoch), CredentialLeaseId: lease, TerminalCols: uint32(cols), TerminalRows: uint32(rows)}}})
	if e != nil {
		_ = connection.Close()
		return nil, e
	}
	first, e := stream.Recv()
	if e != nil || first.GetSequence() != 1 || first.GetReady() == nil || first.GetReady().GetMode() != v.Row.Mode {
		_ = connection.Close()
		return nil, remoteaccess.ErrSessionUnavailable
	}
	return &peerSession{connection: connection, stream: stream, sent: 1, received: 1}, nil
}

type peerSession struct {
	connection     *grpc.ClientConn
	stream         remotev1.KubernetesStreamPeerService_OpenKubernetesStreamClient
	mu             sync.Mutex
	sent, received uint64
	closed         bool
}

func (s *peerSession) Send(_ context.Context, f interactive.Input) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.EOF
	}
	s.sent++
	r := &remotev1.OpenKubernetesStreamRequest{Sequence: s.sent}
	switch f.Type {
	case "input":
		if len(f.Data) > kubernetesstream.MaxFrameBytes {
			return kubernetesstream.ErrProtocol
		}
		r.Frame = &remotev1.OpenKubernetesStreamRequest_Input{Input: &remotev1.GatewayPeerInput{Data: []byte(f.Data)}}
	case "resize":
		r.Frame = &remotev1.OpenKubernetesStreamRequest_Resize{Resize: &remotev1.GatewayPeerResize{Cols: uint32(f.Cols), Rows: uint32(f.Rows)}}
	case "close":
		r.Frame = &remotev1.OpenKubernetesStreamRequest_Close{Close: &remotev1.GatewayPeerClose{Reason: "client_close"}}
	default:
		return kubernetesstream.ErrProtocol
	}
	return s.stream.Send(r)
}
func (s *peerSession) Receive(_ context.Context) (interactive.Frame, error) {
	r, e := s.stream.Recv()
	if e != nil {
		return interactive.Frame{}, e
	}
	if r.GetSequence() != s.received+1 {
		return interactive.Frame{}, kubernetesstream.ErrProtocol
	}
	s.received++
	if o := r.GetOutput(); o != nil && len(o.Data) <= kubernetesstream.MaxFrameBytes {
		return interactive.Frame{Type: "output", Stream: o.Stream, Data: o.Data}, nil
	}
	if f := r.GetState(); f != nil {
		return interactive.Frame{Type: "state", Status: f.Status, Reason: f.Reason}, nil
	}
	return interactive.Frame{}, kubernetesstream.ErrProtocol
}
func (s *peerSession) Close(_ context.Context, _ string) error {
	// Closing the transport also unblocks an outstanding Send; never wait for its mutex first.
	err := s.connection.Close()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return err
}

type PeerServer struct {
	remotev1.UnimplementedKubernetesStreamPeerServiceServer
	Service    kubernetesworkbench.Service
	Factory    Factory
	InstanceID string
}
type peerRead struct {
	request *remotev1.OpenKubernetesStreamRequest
	err     error
}

func (p PeerServer) OpenKubernetesStream(stream remotev1.KubernetesStreamPeerService_OpenKubernetesStreamServer) error {
	initial := make(chan peerRead, 1)
	go func() { first, e := stream.Recv(); initial <- peerRead{first, e} }()
	var first *remotev1.OpenKubernetesStreamRequest
	select {
	case r := <-initial:
		if r.err != nil {
			return r.err
		}
		first = r.request
	case <-time.After(5 * time.Second):
		return status.Error(codes.DeadlineExceeded, "Kubernetes peer hello timeout")
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	open := first.GetOpen()
	if first.Sequence != 1 || open == nil || open.SessionFence < 1 || open.TerminalCols < 20 || open.TerminalCols > 500 || open.TerminalRows < 5 || open.TerminalRows > 300 {
		return status.Error(codes.InvalidArgument, "invalid Kubernetes peer binding")
	}
	id, e := uuid.Parse(open.SessionId)
	if e != nil {
		return status.Error(codes.InvalidArgument, "invalid Kubernetes session")
	}
	v, e := p.Service.CurrentStream(stream.Context(), id, int64(open.SessionFence))
	lease := ""
	if v.Row.CredentialLeaseID.Valid {
		lease = v.Row.CredentialLeaseID.UUID.String()
	}
	if e != nil || v.Row.Status != "connecting" || v.Binding.Route.Mode == "direct" || v.Binding.Route.ConnectorID.String() != open.ConnectorId || uint64(v.Binding.Route.Epoch) != open.ConnectionEpoch || lease != open.CredentialLeaseId {
		return status.Error(codes.PermissionDenied, "Kubernetes stream binding changed")
	}
	owner, e := p.Service.Store.Queries.GetConnectorSession(stream.Context(), db.GetConnectorSessionParams{ConnectorID: v.Binding.Route.ConnectorID, ConnectionEpoch: v.Binding.Route.Epoch})
	if e != nil || owner.GatewayInstanceID != p.InstanceID {
		return status.Error(codes.FailedPrecondition, "Gateway no longer owns Connector")
	}
	ctx, cancel := context.WithDeadline(stream.Context(), v.Row.ExpiresAt.Time)
	defer cancel()
	backend, e := p.Factory.OpenLocal(ctx, v, int(open.TerminalCols), int(open.TerminalRows))
	if e != nil {
		return status.Error(codes.Unavailable, "Kubernetes Connector unavailable")
	}
	defer backend.Close(context.Background(), "peer_closed")
	if e = stream.Send(&remotev1.OpenKubernetesStreamResponse{Sequence: 1, Frame: &remotev1.OpenKubernetesStreamResponse_Ready{Ready: &remotev1.GatewayPeerReady{Mode: v.Row.Mode}}}); e != nil {
		return e
	}
	requests := make(chan peerRead, 1)
	outputs := make(chan backendRead, 1)
	inputs := make(chan interactive.Input, 8)
	sendErrors := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case input := <-inputs:
				if err := backend.Send(ctx, input); err != nil {
					select {
					case sendErrors <- err:
					case <-ctx.Done():
					}
					return
				}
			}
		}
	}()
	go func() {
		for {
			r, err := stream.Recv()
			select {
			case requests <- peerRead{r, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	go readBackend(ctx, backend, outputs)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	sent, received := uint64(1), uint64(1)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sendErrors:
			return err
		case <-ticker.C:
			check, c := context.WithTimeout(ctx, time.Second)
			current, err := p.Service.CurrentStream(check, id, v.Row.Fence)
			c()
			if err != nil || current.Row.ExecutionGateway != p.InstanceID {
				return status.Error(codes.PermissionDenied, "Kubernetes session revoked")
			}
		case r := <-requests:
			if r.err != nil {
				return r.err
			}
			if r.request.Sequence != received+1 {
				return status.Error(codes.InvalidArgument, "Kubernetes peer sequence mismatch")
			}
			received++
			var f interactive.Input
			switch {
			case r.request.GetInput() != nil:
				f = interactive.Input{Type: "input", Data: string(r.request.GetInput().Data)}
				if len(f.Data) > kubernetesstream.MaxFrameBytes {
					return status.Error(codes.InvalidArgument, "frame too large")
				}
			case r.request.GetResize() != nil:
				resize := r.request.GetResize()
				f = interactive.Input{Type: "resize", Cols: int(resize.Cols), Rows: int(resize.Rows)}
			case r.request.GetClose() != nil:
				return nil
			default:
				return status.Error(codes.InvalidArgument, "invalid Kubernetes peer frame")
			}
			select {
			case inputs <- f:
			default:
				return status.Error(codes.ResourceExhausted, "Kubernetes peer input backpressure")
			}
		case r := <-outputs:
			if r.err != nil {
				return r.err
			}
			sent++
			response := &remotev1.OpenKubernetesStreamResponse{Sequence: sent}
			if r.frame.Type == "output" {
				response.Frame = &remotev1.OpenKubernetesStreamResponse_Output{Output: &remotev1.GatewayPeerOutput{Data: r.frame.Data, Stream: r.frame.Stream}}
			} else {
				response.Frame = &remotev1.OpenKubernetesStreamResponse_State{State: &remotev1.GatewayPeerState{Status: r.frame.Status, Reason: r.frame.Reason}}
			}
			if err := stream.Send(response); err != nil {
				return err
			}
			if r.frame.Type == "state" {
				return nil
			}
		}
	}
}
