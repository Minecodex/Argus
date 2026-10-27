package argusidentity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// AuthorizeApplication is consumed by the SDK authentication extension through
// a narrow interface. The established Collector identity owns the mTLS client;
// SDK keys never enter configuration, logs, telemetry attributes or disk.
func (extension *identityExtension) AuthorizeApplication(ctx context.Context, protocol, token string) (map[string]string, error) {
	rejected := errors.New("application ingestion authentication rejected")
	unavailable := status.Error(codes.Unavailable, "application ingestion authentication unavailable")
	endpoint, err := url.Parse(extension.config.ApplicationAuthorizationEndpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || len(token) > 256 {
		return nil, rejected
	}
	certificate, err := tls.LoadX509KeyPair(extension.config.CertificateFile, extension.config.PrivateKeyFile)
	if err != nil {
		return nil, unavailable
	}
	ca, err := os.ReadFile(extension.config.CABundleFile)
	if err != nil {
		return nil, unavailable
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, rejected
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}}}
	defer transport.CloseIdleConnections()
	if extension.config.DialAddress != "" {
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, extension.config.DialAddress)
		}
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return rejected }}
	raw, _ := json.Marshal(map[string]string{"protocol": protocol, "key": token})
	defer clear(raw)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, rejected
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, unavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		if response.StatusCode == http.StatusTooManyRequests {
			return nil, status.Error(codes.ResourceExhausted, "application ingestion authentication rate exceeded")
		}
		if response.StatusCode >= 500 {
			return nil, unavailable
		}
		return nil, rejected
	}
	var result map[string]string
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, rejected
	}
	return result, nil
}
