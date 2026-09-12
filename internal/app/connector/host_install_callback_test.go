package connector

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/anypb"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/sshtarget"
)

func TestBastionHostCallbackFailurePreventsArtifactDownload(t *testing.T) {
	host, port, fingerprint := startSSHServer(t, "connector-password")
	var downloads atomic.Int32
	body := []byte("signed test artifact")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, ca, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGUS_OTELCOL_ARTIFACT_CA_PATH", caPath)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	request := &connectorv1.HostConnectorInstall{
		OperationId: uuid.NewString(), HostId: uuid.NewString(), ConnectorId: uuid.NewString(),
		Address: host, Port: port, Username: "argus", PinnedHostKey: fingerprint,
		TargetPlatform: "linux_arm64", TargetDistributionVersion: "ubuntu:24.04",
		EnrollmentEndpoint: server.URL, GatewayEndpoint: "grpcs://127.0.0.1:9443", TrustBundlePem: ca,
		SigningPublicKey: base64.RawStdEncoding.EncodeToString(public),
		Artifact: &connectorv1.CollectorArtifact{Platform: "linux_arm64", Uri: server.URL,
			Sha256: hex.EncodeToString(digest[:]), ByteSize: uint64(len(body)), SigningKeyId: "test-key",
			Signature: base64.RawStdEncoding.EncodeToString(ed25519.Sign(private, digest[:]))},
	}
	payload, err := anypb.New(request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeHostConnectorInstall(context.Background(), payload, []byte("connector-password"), []byte("unused-token"))
	if sshtarget.CallbackFailureCode(err) != "CONNECT_FAILED" {
		t.Fatalf("expected callback failure, got %v", err)
	}
	if got := downloads.Load(); got != 0 {
		t.Fatalf("downloaded artifact %d times before target callback validation", got)
	}
}
