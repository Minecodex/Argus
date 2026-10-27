package kubernetesstreamgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesaccess"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PreparedOpener interface {
	OpenPrepared(context.Context, uuid.UUID, int64, *connectorv1.RemoteAccessOpen) (interactive.Session, error)
}
type OwnerResolver interface {
	Resolve(context.Context, uuid.UUID, int64) (string, error)
}
type PeerOpener interface {
	Open(context.Context, string, kubernetesworkbench.StreamConnection, int, int) (interactive.Session, error)
}
type BackendFactory interface {
	Open(context.Context, kubernetesworkbench.StreamConnection, int, int) (interactive.Session, error)
}
type Factory struct {
	InstanceID  string
	Service     kubernetesworkbench.Service
	Credentials secret.Service
	Validator   resource.DirectTargetValidator
	Local       PreparedOpener
	Resolver    OwnerResolver
	Peers       PeerOpener
}

func ConnectorRequest(value kubernetesworkbench.StreamConnection, cols, rows int) *connectorv1.RemoteAccessOpen {
	raw, _ := json.Marshal(value.Binding.Target)
	lease := ""
	if value.Row.CredentialLeaseID.Valid {
		lease = value.Row.CredentialLeaseID.UUID.String()
	}
	return &connectorv1.RemoteAccessOpen{SessionId: value.Row.ID.String(), SessionFence: uint64(value.Row.Fence), Protocol: "kubernetes_" + value.Row.Mode,
		CredentialLeaseId: lease, ConnectionEpoch: uint64(value.Binding.Route.Epoch), KubernetesTargetJson: raw,
		ExpiresAt: timestamppb.New(value.Row.ExpiresAt.Time), MaxDuration: durationpb.New(time.Until(value.Row.ExpiresAt.Time)),
		IdleTimeout: durationpb.New(5 * time.Minute), MaxFrameBytes: kubernetesstream.MaxFrameBytes, TerminalCols: uint32(cols), TerminalRows: uint32(rows)}
}
func (f Factory) Open(ctx context.Context, value kubernetesworkbench.StreamConnection, cols, rows int) (interactive.Session, error) {
	if value.Binding.Route.Mode == "direct" {
		if err := f.Service.ClaimStreamExecution(ctx, value.Row.ID, value.Row.Fence, f.InstanceID); err != nil {
			return nil, err
		}
		return f.direct(ctx, value, cols, rows)
	}
	if f.Resolver == nil || f.Local == nil || f.InstanceID == "" {
		return nil, kubernetesinventory.ErrUnavailable
	}
	owner, err := f.Resolver.Resolve(ctx, value.Binding.Route.ConnectorID, value.Binding.Route.Epoch)
	if err != nil {
		return nil, err
	}
	if owner != f.InstanceID {
		if f.Peers == nil {
			return nil, kubernetesinventory.ErrUnavailable
		}
		return f.Peers.Open(ctx, owner, value, cols, rows)
	}
	return f.OpenLocal(ctx, value, cols, rows)
}
func (f Factory) OpenLocal(ctx context.Context, value kubernetesworkbench.StreamConnection, cols, rows int) (interactive.Session, error) {
	if err := f.Service.ClaimStreamExecution(ctx, value.Row.ID, value.Row.Fence, f.InstanceID); err != nil {
		return nil, err
	}
	return f.Local.OpenPrepared(ctx, value.Binding.Route.ConnectorID, value.Binding.Route.Epoch, ConnectorRequest(value, cols, rows))
}
func (f Factory) direct(ctx context.Context, value kubernetesworkbench.StreamConnection, cols, rows int) (interactive.Session, error) {
	if !value.Row.CredentialLeaseID.Valid || value.Row.OwnerGateway != f.InstanceID {
		return nil, secret.ErrInvalidLease
	}
	issued, err := f.Credentials.FulfillLease(secret.WithActorType(ctx, "system"), value.Row.EnterpriseID, value.Row.CredentialLeaseID.UUID, "connector_gateway", f.InstanceID)
	if err != nil {
		return nil, err
	}
	defer clear(issued.Value)
	cfg, err := kubernetesaccess.Config(issued.Value, value.Binding.Target.APIServer)
	if err != nil {
		return nil, err
	}
	target, addresses, err := f.Validator.ResolveHTTPS(ctx, value.Binding.Target.APIServer)
	if err != nil {
		return nil, err
	}
	port := target.Port()
	if port == "" {
		port = "443"
	}
	cfg.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	cfg.Dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		if err := f.Validator.Revalidate(ctx, target.Hostname(), addresses); err != nil {
			return nil, err
		}
		connection, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(addresses[0].String(), port))
		if err != nil {
			return nil, fmt.Errorf("Kubernetes connection unavailable")
		}
		return connection, nil
	}
	return kubernetesstream.Open(ctx, cfg, value.Binding.Target, cols, rows)
}
