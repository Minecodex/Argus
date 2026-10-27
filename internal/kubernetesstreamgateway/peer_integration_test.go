package kubernetesstreamgateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	remotev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/remoteaccess/v1"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/remoteaccess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type peerEndpoint string

func (p peerEndpoint) ResolveEndpoint(context.Context, string, string) (string, error) {
	return string(p), nil
}

type preparedEcho struct{}

func (preparedEcho) OpenPrepared(ctx context.Context, _ uuid.UUID, epoch int64, r *connectorv1.RemoteAccessOpen) (interactive.Session, error) {
	var target kubernetesstream.Target
	if epoch != 1 || r.SessionFence < 1 || json.Unmarshal(r.KubernetesTargetJson, &target) != nil || target.Validate() != nil || r.Protocol != "kubernetes_"+target.Mode {
		return nil, kubernetesstream.ErrProtocol
	}
	return &echoStream{ctx: ctx, mode: target.Mode, events: make(chan interactive.Frame, 16), done: make(chan struct{})}, nil
}
func peerCertificates(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Plan8 owned test CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, public, key)
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(issuer)
	leaf := func(serial int64, usage x509.ExtKeyUsage) tls.Certificate {
		pub, private, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "peer.example.test"}, DNSNames: []string{"peer.example.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		encoded, e := x509.CreateCertificate(rand.Reader, template, issuer, pub, key)
		if e != nil {
			t.Fatal(e)
		}
		return tls.Certificate{Certificate: [][]byte{encoded, der}, PrivateKey: private}
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{leaf(2, x509.ExtKeyUsageServerAuth)}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{leaf(3, x509.ExtKeyUsageClientAuth)}, RootCAs: roots, ServerName: "peer.example.test"}
}
func TestPostgresKubernetesPeerMTLSBindingReplayAndRevocation(t *testing.T) {
	f := newGatewayFixture(t)
	ticket := f.ticket(t, "port_forward")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	v, e := f.service.ClaimStream(ctx, ticket.Session.ID, ticket.Ticket, "gateway-a")
	if e != nil {
		t.Fatal(e)
	}
	f.exec("UPDATE connector_sessions SET gateway_instance_id='gateway-b' WHERE connector_id=$1", v.Binding.Route.ConnectorID)
	serverTLS, clientTLS := peerCertificates(t)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.MaxRecvMsgSize(64<<10), grpc.MaxSendMsgSize(64<<10))
	defer server.Stop()
	remotev1.RegisterKubernetesStreamPeerServiceServer(server, PeerServer{Service: f.service, InstanceID: "gateway-b", Factory: Factory{Service: f.service, InstanceID: "gateway-b", Local: preparedEcho{}}})
	go func() { _ = server.Serve(listener) }()
	dialer := PeerDialer{Transport: &remoteaccess.GatewayPeerDialer{TLSCredentials: credentials.NewTLS(clientTLS), Resolver: peerEndpoint(listener.Addr().String())}}
	wrong := v
	wrong.Binding.Route.Epoch = 2
	if _, e = dialer.Open(ctx, "gateway-b", wrong, 100, 30); e == nil {
		t.Fatal("peer accepted substituted epoch")
	}
	backend, e := dialer.Open(ctx, "gateway-b", v, 100, 30)
	if e != nil {
		t.Fatal(e)
	}
	defer backend.Close(context.Background(), "test_cleanup")
	if _, e = dialer.Open(ctx, "gateway-b", v, 100, 30); e == nil {
		t.Fatal("peer replay opened second backend")
	}
	if e = f.service.ActivateStream(ctx, v.Row.ID, v.Row.Fence, "gateway-a"); e != nil {
		t.Fatal(e)
	}
	if e = backend.Send(ctx, interactive.Input{Type: "input", Data: string(kubernetesstream.Packet(kubernetesstream.ChannelData, 1, []byte{0, 255, 128}))}); e != nil {
		t.Fatal(e)
	}
	frame, e := backend.Receive(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, id, data, e := kubernetesstream.ParsePacket(frame.Data)
	if e != nil || id != 1 || string(data) != string([]byte{0, 255, 128}) {
		t.Fatal("peer changed binary bytes", e)
	}
	badTLS := clientTLS.Clone()
	badTLS.Certificates = nil
	denied := PeerDialer{Transport: &remoteaccess.GatewayPeerDialer{TLSCredentials: credentials.NewTLS(badTLS), Resolver: peerEndpoint(listener.Addr().String())}}
	short, stop := context.WithTimeout(ctx, 1500*time.Millisecond)
	if _, e = denied.Open(short, "gateway-b", v, 100, 30); e == nil {
		t.Fatal("peer accepted missing client certificate")
	}
	stop()
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", f.actor.EnterpriseID, f.namespace)
	if _, e = backend.Receive(ctx); e == nil {
		t.Fatal("peer survived Namespace revocation")
	}
}
