package sshtarget

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/tlsmaterial"
)

// CallbackError contains only errors from the unauthenticated network probe.
// No enrollment token, SSH credential, or remote installation output is used.
type CallbackError struct {
	Code  string
	cause error
}

func (err *CallbackError) Error() string {
	if err.cause == nil {
		return "Connector callback " + err.Code
	}
	return "Connector callback " + err.Code + ": " + err.cause.Error()
}
func (err *CallbackError) Unwrap() error { return err.cause }

func CallbackFailureCode(err error) string {
	var failure *CallbackError
	if errors.As(err, &failure) {
		return failure.Code
	}
	return ""
}

// CallbackFailureDetail is safe to log; arbitrary remote stderr is not.
func CallbackFailureDetail(err error) string {
	var failure *CallbackError
	if errors.As(err, &failure) {
		return failure.Error()
	}
	return ""
}

// ProbeOnboarding checks the frozen enrollment origin and Gateway server
// identity through the target's SSH transport. It does not enroll a Connector.
func ProbeOnboarding(ctx context.Context, client *ssh.Client, plan installation.CallbackProbePlan) error {
	if plan.TrustBundleEpoch <= 0 {
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("callback Trust Bundle epoch is required")}
	}
	switch plan.ControlPath {
	case "direct":
		if plan.EnrollDialAddress != "" || plan.GatewayDialAddress != "" {
			return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("direct callbacks must use their public origins")}
		}
	case "executor_tunnel":
		for _, address := range []string{plan.EnrollDialAddress, plan.GatewayDialAddress} {
			host, _, err := net.SplitHostPort(address)
			if err != nil || host != "127.0.0.1" {
				return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("executor callbacks require loopback forwards")}
			}
		}
	case "bastion_relay":
		if plan.EnrollDialAddress == "" || plan.GatewayDialAddress == "" || plan.RelayPortGeneration <= 0 {
			return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("Bastion callback listeners and generation are required")}
		}
	default:
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("callback control path is invalid")}
	}
	if err := ProbeEnrollment(ctx, client, plan.EnrollmentEndpoint, plan.EnrollDialAddress, plan.TrustBundlePEM); err != nil {
		return err
	}
	return ProbeGateway(ctx, client, plan.GatewayEndpoint, plan.GatewayDialAddress, plan.TrustBundlePEM)
}

// ProbeGateway verifies TLS 1.3, the frozen server CA/SNI and gRPC ALPN. Before
// enrollment there is no client certificate: this proves server reachability
// and identity, not successful mTLS registration or an online Connector.
func ProbeGateway(ctx context.Context, client *ssh.Client, endpoint, dialAddress string, caPEM []byte) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || client == nil || parsed.Scheme != "grpcs" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("Gateway origin must be a grpcs origin")}
	}
	if dialAddress == "" {
		port := parsed.Port()
		if port == "" {
			port = "443"
		}
		dialAddress = net.JoinHostPort(parsed.Hostname(), port)
	}
	if host, port, err := net.SplitHostPort(dialAddress); err != nil || host == "" || port == "" {
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("Gateway dial target must be host:port")}
	}
	configuration, err := tlsmaterial.StaticClientConfig(caPEM, nil, nil, parsed.Hostname())
	if err != nil {
		return &CallbackError{Code: "TLS_FAILED", cause: err}
	}
	configuration.NextProtos = []string{"h2"}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	connection, err := client.DialContext(probeCtx, "tcp", dialAddress)
	if err != nil {
		if probeCtx.Err() != nil {
			return callbackTransportError(probeCtx.Err())
		}
		return callbackTransportError(err)
	}
	defer connection.Close()
	secure := tls.Client(connection, configuration)
	if err := secure.HandshakeContext(probeCtx); err != nil {
		if probeCtx.Err() != nil {
			return callbackTransportError(probeCtx.Err())
		}
		return callbackTransportError(err)
	}
	if secure.ConnectionState().NegotiatedProtocol != "h2" {
		return &CallbackError{Code: "TLS_FAILED", cause: errors.New("Gateway did not negotiate gRPC h2")}
	}
	return nil
}

// ProbeEnrollment follows the same target-side SSH path and strict TLS origin
// as enrollment. A remote-forward listener alone can accept TCP connections
// even when the Executor cannot connect to the Argus HTTPS upstream.
func ProbeEnrollment(ctx context.Context, client *ssh.Client, endpoint, dialAddress string, caPEM []byte) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || client == nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("enrollment origin must be an HTTPS origin")}
	}
	if dialAddress == "" {
		port := parsed.Port()
		if port == "" {
			port = "443"
		}
		dialAddress = net.JoinHostPort(parsed.Hostname(), port)
	}
	if host, port, err := net.SplitHostPort(dialAddress); err != nil || host == "" || port == "" {
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("enrollment dial target must be host:port")}
	}
	tlsConfig, err := tlsmaterial.StaticClientConfig(caPEM, nil, nil, parsed.Hostname())
	if err != nil {
		return &CallbackError{Code: "TLS_FAILED", cause: err}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	transport := &http.Transport{
		Proxy: nil, TLSClientConfig: tlsConfig, ForceAttemptHTTP2: true, DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return client.DialContext(ctx, network, dialAddress)
		},
	}
	defer transport.CloseIdleConnections()
	probe := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	parsed.Path = "/api/v1/setup/status"
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return &CallbackError{Code: "CONFIG_INVALID", cause: err}
	}
	response, err := probe.Do(request)
	if err != nil {
		if probeCtx.Err() != nil {
			return callbackTransportError(probeCtx.Err())
		}
		return callbackTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &CallbackError{Code: "HTTP_FAILED", cause: fmt.Errorf("platform status returned HTTP %d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil {
		if probeCtx.Err() != nil {
			return callbackTransportError(probeCtx.Err())
		}
		return callbackTransportError(err)
	}
	var state struct {
		State string `json:"state"`
	}
	if len(body) > 4096 || json.Unmarshal(body, &state) != nil || state.State != "initialized" {
		return &CallbackError{Code: "RESPONSE_INVALID", cause: errors.New("callback did not return an initialized Argus platform")}
	}
	return nil
}

func callbackTransportError(err error) error {
	code := "CONNECT_FAILED"
	var verification *tls.CertificateVerificationError
	var unknownCA x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var dns *net.DNSError
	var network net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		code = "TIMEOUT"
	case errors.As(err, &dns):
		code = "DNS_FAILED"
	case errors.As(err, &verification), errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &invalid):
		code = "TLS_FAILED"
	case errors.As(err, &network) && network.Timeout():
		code = "TIMEOUT"
	case isSSHCallbackDNSFailure(err):
		code = "DNS_FAILED"
	case strings.Contains(err.Error(), "tls:"):
		code = "TLS_FAILED"
	}
	return &CallbackError{Code: code, cause: err}
}

// SSH reports target-side resolver failures as channel-open descriptions,
// instead of preserving the target operating system's net.DNSError type.
func isSSHCallbackDNSFailure(err error) bool {
	var channel *ssh.OpenChannelError
	if !errors.As(err, &channel) || channel.Reason != ssh.ConnectionFailed {
		return false
	}
	detail := strings.ToLower(channel.Message)
	for _, message := range []string{"no such host", "name or service not known", "temporary failure in name resolution", "nodename nor servname", "getaddrinfo failed", "host is unknown"} {
		if strings.Contains(detail, message) {
			return true
		}
	}
	return false
}
