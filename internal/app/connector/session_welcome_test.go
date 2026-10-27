package connector

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"github.com/google/uuid"
	connectorcore "github.com/kakj-go/Argus/internal/connector"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/trustbundle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"
)

type ownedSilentWelcomeServer struct {
	connectorv1.UnimplementedConnectorControlServiceServer
	hello chan struct{}
}

func (s *ownedSilentWelcomeServer) Connect(stream connectorv1.ConnectorControlService_ConnectServer) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	if frame.GetHello() == nil {
		return errors.New("owned peer did not receive hello")
	}
	close(s.hello)
	<-stream.Context().Done()
	return stream.Context().Err()
}

func TestControlHandshakeHasBoundedWelcomeWait(t *testing.T) {
	now := time.Now()
	id := uuid.New()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	store := localStore{directory: t.TempDir()}
	local, err := connectorcore.GenerateLocalIdentity(store.directory, id)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := connectorcore.MarshalPrivateKey(local.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse(connectorcore.CertificateURI(id))
	client := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), URIs: []*url.URL{uri}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	clientDER, err := x509.CreateCertificate(rand.Reader, client, ca, &local.PrivateKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := trustbundle.Parse(caPEM, now)
	if err != nil {
		t.Fatal(err)
	}
	identity := identityState{ConnectorID: id.String(), Role: "host", InstanceID: "owned-welcome-wait", Name: "owned-welcome-wait", GatewayEndpoint: "grpcs://gateway.example.test:9443", EnrollmentEndpoint: "https://control.example.test", CertificateExpiresAt: now.Add(time.Hour), Capabilities: []string{"collector_management"}, TrustBundleEpoch: 1, TrustBundleSHA256: bundle.SHA256, TrustCAFingerprints: bundle.Fingerprints}
	if err = store.saveEnrollmentIdentity(identity, keyPEM, []byte(local.CSRPEM), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}), caPEM); err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(3), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), DNSNames: []string{"gateway.example.test"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	serverDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(caPEM)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	peer := &ownedSilentWelcomeServer{hello: make(chan struct{})}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{certificate}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12})))
	connectorv1.RegisterConnectorControlServiceServer(server, peer)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	t.Setenv("ARGUS_CONNECTOR_DIAL_ADDRESS", listener.Addr().String())
	ctx, cancel := context.WithTimeout(t.Context(), 22*time.Second)
	defer cancel()
	started := time.Now()
	err = (connectorClient{store: store}).runSession(ctx)
	if err == nil || err.Error() != "Connector gateway welcome timeout" || time.Since(started) > 20*time.Second {
		t.Fatalf("welcome wait did not end at the bounded transport deadline: %v", err)
	}
	select {
	case <-peer.hello:
	default:
		t.Fatal("deadline fired without a real verified-mTLS hello")
	}
}
