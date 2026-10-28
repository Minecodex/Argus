package httpapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	telemetryv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/telemetry/v1"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type modelGovernanceLimiter struct{}

func (modelGovernanceLimiter) Acquire(context.Context, uuid.UUID, string, time.Duration) (func(), error) {
	return func() {}, nil
}

// Real loopback gRPC with mutual certificate verification; no auth interceptor
// injects an identity and no transport/metric response is mocked.
func modelGovernanceBackend(t *testing.T, service modelmetrics.Service) *telemetry.GRPCQueryBackend {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	issue := func(serial int64, client bool) tls.Certificate {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		if client {
			uri, _ := url.Parse("spiffe://argus.io/services/server/telemetry-client")
			if serial == 4 {
				uri, _ = url.Parse("spiffe://argus.io/services/worker/telemetry-client")
			}
			template.URIs = []*url.URL{uri}
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		} else {
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}
		data, e := x509.CreateCertificate(rand.Reader, template, ca, &k.PublicKey, key)
		if e != nil {
			t.Fatal(e)
		}
		return tls.Certificate{Certificate: [][]byte{data}, PrivateKey: k}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{issue(2, false)}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert})))
	telemetry.RegisterModelGovernanceRPC(server, service, modelGovernanceLimiter{})
	publicRouter := telemetry.TenantTableRouter{}
	publicEngine := promqlengine.NewEngine(service.Conn, publicRouter, nil)
	telemetry.RegisterQueryRPC(server, telemetry.ClickHouseQuery{Conn: service.Conn, Router: publicRouter, PromQL: publicEngine}, modelGovernanceLimiter{}, nil, []*queryengine.Coordinator{{PromQL: queryengine.PromQLEngine{Engine: publicEngine}}}, nil)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	worker, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{issue(4, true)}})))
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	_, err = telemetryv1.NewModelGovernanceServiceClient(worker).QueryModelGovernance(t.Context(), &telemetryv1.QueryModelGovernanceRequest{SchemaVersion: modelmetrics.ProtocolVersion, RequestJson: []byte(`{}`)})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal("worker identity reached private model RPC", err)
	}
	backend, err := telemetry.NewGRPCQueryBackend("grpcs://"+listener.Addr().String(), credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{issue(3, true)}}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}
