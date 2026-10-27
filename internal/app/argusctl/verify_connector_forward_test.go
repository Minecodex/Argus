package argusctl

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalConnectorForwardVerifiesTLSIdentityWithoutWeakeningNormalExposure(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &InstallConfig{}
	cfg.Spec.Profile = "evaluation"
	cfg.Spec.KubeContext = "docker-desktop"
	cfg.Spec.Images.Mode = "local-registry"
	cfg.Spec.Exposure.ConnectorHost = "example.com"
	address := strings.TrimPrefix(server.URL, "https://")
	for _, profile := range []string{"evaluation", "local-hardening"} {
		for _, name := range []string{"docker-desktop", "kind-plan8-local", "renamed-local-lab"} {
			t.Run(profile+"/"+name, func(t *testing.T) {
				cfg.Spec.Profile, cfg.Spec.KubeContext = profile, name
				if err := verifyConnectorForward(context.Background(), cfg, address, path, server.TLS.Certificates[0]); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	if err := verifyConnectorForward(context.Background(), cfg, address, path); err == nil {
		t.Fatal("server identity without client authentication accepted")
	}
	cfg.Spec.Exposure.ConnectorHost = "wrong.example.test"
	if err := verifyConnectorForward(context.Background(), cfg, address, path); err == nil {
		t.Fatal("wrong service identity accepted")
	}
	cfg.Spec.Exposure.ConnectorHost = "example.com"
	for _, profile := range []string{"production", "", "unknown"} {
		cfg.Spec.Profile = profile
		if err := verifyConnectorForward(context.Background(), cfg, address, path, server.TLS.Certificates[0]); err == nil || !strings.Contains(err.Error(), "requires an evaluation or local-hardening profile") {
			t.Fatalf("unsupported profile %q replaced normal exposure verification: %v", profile, err)
		}
	}
	cfg.Spec.Profile = "evaluation"
	cfg.Spec.Images.Mode = "remote-registry"
	if err := verifyConnectorForward(context.Background(), cfg, address, path, server.TLS.Certificates[0]); err == nil {
		t.Fatal("non-local image mode accepted")
	}
	cfg.Spec.Images.Mode = "local-registry"
	if err := verifyConnectorForward(context.Background(), cfg, "192.0.2.1:9443", path, server.TLS.Certificates[0]); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-local target accepted or rejected for the wrong reason: %v", err)
	}
}

func TestLocalConnectorForwardLiveIdentity(t *testing.T) {
	path, address := os.Getenv("ARGUS_CONNECTOR_FORWARD_TEST_CONFIG"), os.Getenv("ARGUS_CONNECTOR_FORWARD_TEST_ADDRESS")
	if path == "" || address == "" {
		t.Skip("explicit read-only local verification target not configured")
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	clients, err := clientsFor(cfg.Spec.KubeContext)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := materializeTrustBundle(ctx, clients, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(ca)
	if err := verifyOwnedConnectorForward(ctx, clients, cfg, address, ca); err != nil {
		t.Fatal(err)
	}
}
