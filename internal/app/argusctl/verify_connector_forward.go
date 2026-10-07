package argusctl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"golang.org/x/net/http2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
	"os"
	"time"
)

// The local E2E runner explicitly proves its port-forward TLS route. This is
// reported separately and never represents external LoadBalancer acceptance.
func verifyConnectorForward(ctx context.Context, cfg *InstallConfig, address, caPath string, certificates ...tls.Certificate) error {
	if cfg.Spec.Profile == "production" || cfg.Spec.KubeContext != "docker-desktop" || cfg.Spec.Images.Mode != "local-registry" {
		return fmt.Errorf("connector forward verification is restricted to local Docker Desktop profiles")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("connector forward must be an explicit loopback address and port")
	}
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("connector forward CA is invalid")
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: &tls.Config{RootCAs: pool, ServerName: cfg.Spec.Exposure.ConnectorHost, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2"}, Certificates: certificates}}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connector forward TLS identity: %w", err)
	}
	defer connection.Close()
	state := connection.(*tls.Conn).ConnectionState()
	if len(state.VerifiedChains) == 0 || state.NegotiatedProtocol != "h2" {
		return fmt.Errorf("connector forward has no verified HTTP/2 TLS identity")
	}
	// TLS 1.3 can report the server identity before a server rejects missing
	// client authentication. Require an actual HTTP/2 settings exchange too.
	deadline := time.Now().Add(10 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return err
	}
	if _, err := connection.Write([]byte(http2.ClientPreface)); err != nil {
		return err
	}
	framer := http2.NewFramer(connection, connection)
	if err := framer.WriteSettings(); err != nil {
		return err
	}
	frame, err := framer.ReadFrame()
	if err != nil {
		return fmt.Errorf("connector forward authenticated HTTP/2: %w", err)
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return fmt.Errorf("connector forward did not acknowledge HTTP/2 settings")
	}
	return nil
}

func verifyOwnedConnectorForward(ctx context.Context, clients *kubeClients, cfg *InstallConfig, address, caPath string) error {
	secret, err := clients.typed.CoreV1().Secrets(cfg.Spec.Namespaces.System).Get(ctx, "argus-connector-gateway-peer-client-tls", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("connector verification identity: %w", err)
	}
	pair, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
	if err != nil {
		return fmt.Errorf("connector verification identity: %w", err)
	}
	return verifyConnectorForward(ctx, cfg, address, caPath, pair)
}
