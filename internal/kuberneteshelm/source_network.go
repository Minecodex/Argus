package kuberneteshelm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kakj-go/Argus/internal/resource"
)

type sourceSession struct {
	client    *http.Client
	transport *http.Transport
	policy    ChartNetworkPolicy
	origin    *url.URL
	authURL   *url.URL
	plain     bool
	mu        sync.Mutex
	addresses map[string][]netip.Addr
	requests  int
	remaining int64
}

func newSourceSession(s ChartSource, c SourceCredentials, policy ChartNetworkPolicy) (*sourceSession, error) {
	u := s.URL
	if s.Kind == "oci" {
		scheme := "https://"
		if s.PlainHTTP {
			scheme = "http://"
		}
		u = scheme + strings.TrimPrefix(u, "oci://")
	}
	origin, e := sourceURL(u, s.PlainHTTP)
	if e != nil {
		return nil, e
	}
	if policy == nil {
		policy = resource.DirectTargetValidator{}
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAPEM != "" {
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(c.CAPEM)) {
			return nil, ErrInvalid
		}
		tlsConfig.RootCAs = pool
	}
	n := &sourceSession{policy: policy, origin: origin, plain: s.PlainHTTP, addresses: map[string][]netip.Addr{}, remaining: 4 << 20}
	if s.AuthURL != "" {
		n.authURL, e = sourceURL(s.AuthURL, s.PlainHTTP)
		if e != nil {
			return nil, e
		}
	}
	n.transport = &http.Transport{Proxy: nil, TLSClientConfig: tlsConfig, DialContext: n.dial, DisableCompression: true, MaxResponseHeaderBytes: 32 << 10, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxIdleConnsPerHost: 2}
	n.client = &http.Client{Transport: n, Timeout: 20 * time.Second, CheckRedirect: func(next *http.Request, previous []*http.Request) error {
		if len(previous) > 3 || len(previous) == 0 {
			return ErrDenied
		}
		last := previous[len(previous)-1]
		if last.URL.Scheme == "https" && next.URL.Scheme != "https" {
			return ErrDenied
		}
		if sourceOrigin(next.URL) != sourceOrigin(last.URL) {
			next.Header.Del("Authorization")
			next.Header.Del("Proxy-Authorization")
			next.Header.Del("Cookie")
		}
		return nil
	}}
	return n, nil
}

func sourceOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func (n *sourceSession) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead || r.URL.User != nil || r.URL.Hostname() == "" || r.URL.Scheme != "https" && !(n.plain && r.URL.Scheme == "http") {
		return nil, ErrDenied
	}
	if n.origin.Scheme == "https" && r.URL.Scheme != "https" {
		return nil, ErrDenied
	}
	if r.Header.Get("Authorization") != "" && sourceOrigin(r.URL) != sourceOrigin(n.origin) && (n.authURL == nil || sourceOrigin(r.URL) != sourceOrigin(n.authURL) || r.URL.EscapedPath() != n.authURL.EscapedPath()) {
		return nil, ErrDenied
	}
	n.mu.Lock()
	n.requests++
	requests, remaining := n.requests, n.remaining
	addresses := n.addresses[r.URL.Hostname()]
	n.mu.Unlock()
	if requests > 32 || remaining <= 0 {
		return nil, ErrBudget
	}
	if addresses == nil {
		var e error
		addresses, e = n.policy.Resolve(r.Context(), r.URL.Hostname())
		if e != nil || len(addresses) == 0 {
			return nil, ErrDenied
		}
		n.mu.Lock()
		n.addresses[r.URL.Hostname()] = addresses
		n.mu.Unlock()
	}
	if e := n.policy.Revalidate(r.Context(), r.URL.Hostname(), addresses); e != nil {
		return nil, ErrDenied
	}
	response, e := n.transport.RoundTrip(r)
	if e != nil {
		return nil, ErrUnavailable
	}
	if response.ContentLength > 2<<20 || response.ContentLength > remaining {
		response.Body.Close()
		return nil, ErrBudget
	}
	response.Body = &sourceBody{ReadCloser: response.Body, session: n}
	return response, nil
}

func (n *sourceSession) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	p, pe := strconv.Atoi(port)
	if e != nil || pe != nil || p < 1 || p > 65535 {
		return nil, ErrDenied
	}
	n.mu.Lock()
	addresses := n.addresses[host]
	n.mu.Unlock()
	if len(addresses) == 0 || n.policy.Revalidate(ctx, host, addresses) != nil {
		return nil, ErrDenied
	}
	for _, ip := range addresses {
		conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
	}
	return nil, ErrUnavailable
}

type sourceBody struct {
	io.ReadCloser
	session *sourceSession
}

func (b *sourceBody) Read(p []byte) (int, error) {
	b.session.mu.Lock()
	remaining := b.session.remaining
	b.session.mu.Unlock()
	if remaining <= 0 {
		return 0, ErrBudget
	}
	if int64(len(p)) > remaining+1 {
		p = p[:remaining+1]
	}
	n, e := b.ReadCloser.Read(p)
	b.session.mu.Lock()
	b.session.remaining -= int64(n)
	remaining = b.session.remaining
	b.session.mu.Unlock()
	if remaining < 0 {
		return 0, ErrBudget
	}
	return n, e
}
