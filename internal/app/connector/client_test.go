package connector

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/collectormanager"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/telemetrybinding"
	"github.com/kakj-go/Argus/internal/tlsmaterial"
	"github.com/kakj-go/Argus/internal/trustbundle"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEnrollmentEndpointRequiresHTTPSOutsideLoopback(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{name: "https base", input: "https://control.example.test", want: "https://control.example.test/api/v1/connectors/enroll"},
		{name: "existing path", input: "https://control.example.test/api/v1/connectors/enroll", want: "https://control.example.test/api/v1/connectors/enroll"},
		{name: "loopback http", input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080/api/v1/connectors/enroll"},
		{name: "remote http", input: "http://control.example.test", wantError: true},
		{name: "missing host", input: "https://", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := enrollmentEndpoint(test.input)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected %q to be rejected", test.input)
				}
				return
			}
			if err != nil || value != test.want {
				t.Fatalf("endpoint=%q err=%v, want %q", value, err, test.want)
			}
		})
	}
}

func TestPinnedEnrollmentAddressOnlyOverridesDialing(t *testing.T) {
	transport := pinnedAddressTransport("127.0.0.1:8443")
	if transport.DialContext == nil || transport.TLSClientConfig != nil {
		t.Fatalf("pinned base transport must leave TLS identity to tlsmaterial: %#v", transport)
	}
}

func TestPinnedEnrollmentClientHelloCarriesRelayALPN(t *testing.T) {
	certificateSource := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certificateSource.Close()
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateSource.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := tlsmaterial.Load(tlsmaterial.Options{CABundlePath: caPath})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	evidence := make(chan []string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			evidence <- nil
			return
		}
		defer connection.Close()
		_, _, alpn, _ := readTLSClientHello(connection)
		evidence <- alpn
	}()
	transport, err := tlsmaterial.NewHTTPTransport(material, pinnedAddressTransport(listener.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: time.Second}
	_, _ = client.Get("https://argus.example.test/api/v1/connectors/enroll")
	if alpn := <-evidence; !containsAllowedALPN(alpn, map[string]bool{"h2": true, "http/1.1": true}) {
		t.Fatalf("pinned enrollment ClientHello omitted relay ALPN: %v", alpn)
	}
}

func TestParseGatewayEndpointRequiresGRPCSTarget(t *testing.T) {
	for _, value := range []string{"gateway.example.test:9443", "grpcs://gateway.example.test:9443"} {
		parsed, err := parseGatewayEndpoint(value)
		if err != nil || parsed.Host != "gateway.example.test:9443" {
			t.Fatalf("parse %q: endpoint=%v err=%v", value, parsed, err)
		}
	}
	for _, value := range []string{"grpc://gateway.example.test:9443", "grpcs://gateway.example.test", "grpcs://gateway.example.test:9443/path"} {
		if _, err := parseGatewayEndpoint(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestLocalStoreUsesPrivateAtomicFilesAndPrunesResults(t *testing.T) {
	store := localStore{directory: filepath.Join(t.TempDir(), "connector")}
	identity := identityState{ConnectorID: "018f47e2-9a4c-7b31-8acd-02a2475e8d2f", Role: "bastion", InstanceID: "instance-1",
		Name: "bastion-1", GatewayEndpoint: "grpcs://gateway.example.test:9443", CertificateExpiresAt: time.Now().Add(time.Hour),
		Capabilities: []string{"host.connection_probe"}}
	if err := store.saveIdentity(identity, []byte("key"), []byte("certificate"), []byte("ca")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{identityFile, keyFile, certFile, caFile} {
		info, err := os.Stat(filepath.Join(store.directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode=%o, want 600", name, info.Mode().Perm())
		}
		if _, err := os.Stat(filepath.Join(store.directory, name+".tmp")); !os.IsNotExist(err) {
			t.Fatalf("temporary file for %s was not removed", name)
		}
	}
	seed, err := json.Marshal(map[string]commandRecord{
		"old": {CommandID: "old", UpdatedAt: time.Now().Add(-25 * time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicPrivateWrite(filepath.Join(store.directory, resultsFile), seed); err != nil {
		t.Fatal(err)
	}
	if err := store.saveResult(commandRecord{CommandID: "current", IdempotencyKey: "idem-1", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	results, err := store.loadResults()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := results["old"]; ok {
		t.Fatal("expired command result was retained")
	}
	if _, ok := findRecordedResult(results, &connectorv1.ConnectorCommand{CommandId: "retry", IdempotencyKey: "idem-1"}); !ok {
		t.Fatal("idempotency-key retry did not reuse the recorded result")
	}
}

func TestEnrollmentIdentityCommitClearsPreviousConnectorRuntimeState(t *testing.T) {
	store := localStore{directory: filepath.Join(t.TempDir(), "connector")}
	oldIdentity := identityState{ConnectorID: uuid.NewString(), Role: "bastion", InstanceID: "machine-1", Name: "old",
		EnrollmentEndpoint: "https://old.example.test", GatewayEndpoint: "grpcs://old.example.test:9443",
		CertificateExpiresAt: time.Now().Add(time.Hour), Capabilities: []string{"bastion.tls_relay"}, TrustBundleEpoch: 1,
		TrustBundleSHA256: strings.Repeat("a", 64), TrustCAFingerprints: []string{strings.Repeat("b", 64)}}
	if err := store.saveIdentity(oldIdentity, []byte("old-key"), []byte("old-certificate"), []byte("old-ca")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{resultsFile, relayFile} {
		if err := atomicPrivateWrite(filepath.Join(store.directory, name), []byte(`{"old":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	nextIdentity := oldIdentity
	nextIdentity.ConnectorID = uuid.NewString()
	nextIdentity.Name = "next"
	if err := store.saveEnrollmentIdentity(nextIdentity, []byte("next-key"), []byte("next-csr"), []byte("next-certificate"), []byte("next-ca")); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.loadIdentity()
	if err != nil || loaded.ConnectorID != nextIdentity.ConnectorID {
		t.Fatalf("new Connector identity did not commit: %+v, %v", loaded, err)
	}
	csr, err := os.ReadFile(filepath.Join(store.directory, "connector.csr.pem"))
	if err != nil || string(csr) != "next-csr" {
		t.Fatalf("new enrollment CSR was not committed: %q, %v", csr, err)
	}
	for _, name := range []string{resultsFile, relayFile} {
		if _, err := os.Stat(filepath.Join(store.directory, name)); !os.IsNotExist(err) {
			t.Fatalf("previous Connector runtime state %s was retained", name)
		}
	}
}

func TestTrustBundleUpdateRejectsStaleEpochAndPersistsValidatedBundle(t *testing.T) {
	material := connectorTestTrustBundle(t)
	identity := identityState{ConnectorID: "018f47e2-9a4c-7b31-8acd-02a2475e8d2f", Role: "host", InstanceID: "instance-1",
		Name: "host-1", GatewayEndpoint: "grpcs://gateway.example.test:9443", CertificateExpiresAt: time.Now().Add(time.Hour),
		Capabilities: []string{"host.local_command"}, TrustBundleEpoch: 2, TrustBundleSHA256: material.SHA256,
		TrustCAFingerprints: append([]string(nil), material.Fingerprints...)}
	client := connectorClient{store: localStore{directory: t.TempDir()}}
	update := &connectorv1.TrustBundleUpdate{Epoch: 1, State: trustbundle.StateStable, BundlePem: material.PEM,
		BundleSha256: material.SHA256, CurrentCaFingerprints: material.Fingerprints, StartedAt: timestamppb.Now()}
	if _, _, err := client.acceptTrustBundle(identity, update); err == nil {
		t.Fatal("stale Trust Bundle epoch was accepted")
	}
	update.Epoch = 3
	update.BundleSha256 = strings.Repeat("0", 64)
	if _, _, err := client.acceptTrustBundle(identity, update); err == nil {
		t.Fatal("Trust Bundle with a mismatched digest was accepted")
	}
	update.BundleSha256 = material.SHA256
	next, acknowledgement, err := client.acceptTrustBundle(identity, update)
	if err != nil {
		t.Fatal(err)
	}
	if next.TrustBundleEpoch != 3 || acknowledgement.GetEpoch() != 3 || acknowledgement.GetBundleSha256() != material.SHA256 {
		t.Fatalf("unexpected Trust Bundle acknowledgement: identity=%+v ack=%+v", next, acknowledgement)
	}
	stored, err := os.ReadFile(filepath.Join(client.store.directory, caFile))
	if err != nil || string(stored) != string(material.PEM) {
		t.Fatalf("validated Trust Bundle was not persisted: %v", err)
	}
}

func connectorTestTrustBundle(t *testing.T) trustbundle.Material {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(99), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	encoded, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	material, err := trustbundle.Parse(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded}), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return material
}

func TestCertificateNeedsRotationAtTwoThirdsTTL(t *testing.T) {
	writeCertificate := func(t *testing.T, notBefore, notAfter time.Time) localStore {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: notBefore, NotAfter: notAfter}
		encoded, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		store := localStore{directory: t.TempDir()}
		if err := store.ensure(); err != nil {
			t.Fatal(err)
		}
		certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded})
		for name, value := range map[string][]byte{certFile: certificate, keyFile: []byte("key"), caFile: []byte("ca")} {
			if err := atomicPrivateWrite(filepath.Join(store.directory, name), value); err != nil {
				t.Fatal(err)
			}
		}
		return store
	}
	now := time.Now()
	if certificateNeedsRotation(writeCertificate(t, now.Add(-10*time.Minute), now.Add(50*time.Minute))) {
		t.Fatal("fresh certificate requested rotation before two thirds of its TTL")
	}
	if !certificateNeedsRotation(writeCertificate(t, now.Add(-50*time.Minute), now.Add(10*time.Minute))) {
		t.Fatal("certificate did not request rotation after two thirds of its TTL")
	}
}

func TestCommandExecutorRejectsInvalidTypesAndAcceptsUninstall(t *testing.T) {
	expires := timestamppb.New(time.Now().Add(time.Minute))
	unknown, err := anypb.New(&connectorv1.ConnectorUninstall{})
	if err != nil {
		t.Fatal(err)
	}
	outcome := (commandExecutor{}).execute(context.Background(), &connectorv1.ConnectorCommand{
		CommandId: "command-1", CommandType: "arbitrary_shell", ExpiresAt: expires, TypedPayload: unknown,
	}, nil, nil)
	if outcome.code != "CONNECTOR_COMMAND_FAILED" || outcome.stop {
		t.Fatalf("unexpected unknown-command outcome: %+v", outcome)
	}
	uninstall, err := anypb.New(&connectorv1.ConnectorUninstall{})
	if err != nil {
		t.Fatal(err)
	}
	outcome = (commandExecutor{}).execute(context.Background(), &connectorv1.ConnectorCommand{
		CommandId: "command-2", CommandType: "connector_uninstall", ExpiresAt: expires, TypedPayload: uninstall,
	}, nil, nil)
	if outcome.code != "" || !outcome.stop || outcome.result == nil {
		t.Fatalf("unexpected uninstall outcome: %+v", outcome)
	}
	expired := (commandExecutor{}).execute(context.Background(), &connectorv1.ConnectorCommand{
		CommandId: "command-3", CommandType: "connector_uninstall", ExpiresAt: timestamppb.New(time.Now().Add(-time.Second)), TypedPayload: uninstall,
	}, nil, nil)
	if expired.code != "CONNECTOR_COMMAND_INVALID" {
		t.Fatalf("expired command code=%q", expired.code)
	}
}

func TestCollectorManagementUsesConvergenceTimeout(t *testing.T) {
	if got := timeoutForCommand("host_connection_probe"); got != 45*time.Second {
		t.Fatalf("host command timeout=%s, want 45s", got)
	}
	if got := timeoutForCommand("collector_management"); got != 3*time.Minute {
		t.Fatalf("Collector command timeout=%s, want 3m", got)
	}
}

func TestCollectorManagementFailureCodesAreStableAndSanitized(t *testing.T) {
	for name, test := range map[string]struct {
		err  error
		code string
	}{
		"invalid":  {collectormanager.ErrInvalidCommand, "COLLECTOR_COMMAND_INVALID"},
		"artifact": {collectormanager.ErrArtifactInvalid, "COLLECTOR_ARTIFACT_INVALID"},
		"evidence": {telemetrybinding.ErrInvalidNodeEvidence, "COLLECTOR_NODE_EVIDENCE_INVALID"},
		"timeout":  {context.DeadlineExceeded, "COLLECTOR_HEALTH_CHECK_FAILED"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := collectorManagementFailureCode(test.err); got != test.code {
				t.Fatalf("failure code=%q, want %q", got, test.code)
			}
		})
	}
}

func TestStopSequenceRequiresAcknowledgementOfUninstallResult(t *testing.T) {
	for _, test := range []struct {
		name         string
		stopSequence uint64
		acknowledged uint64
		want         bool
	}{
		{name: "not stopping", stopSequence: 0, acknowledged: 12, want: false},
		{name: "earlier frame", stopSequence: 12, acknowledged: 11, want: false},
		{name: "uninstall result", stopSequence: 12, acknowledged: 12, want: true},
		{name: "later cumulative acknowledgement", stopSequence: 12, acknowledged: 13, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := stopSequenceAcknowledged(test.stopSequence, test.acknowledged); got != test.want {
				t.Fatalf("stopSequenceAcknowledged(%d, %d)=%v, want %v", test.stopSequence, test.acknowledged, got, test.want)
			}
		})
	}
}
