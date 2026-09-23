package remotemcp

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
)

// EndpointPolicy allows customer private networks but excludes loopback,
// metadata, non-unicast addresses and operator-protected infrastructure.
type EndpointPolicy struct {
	Resolver    modelprovider.Resolver
	DeniedCIDRs []netip.Prefix
}

func (policy EndpointPolicy) Validate(ctx context.Context, raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, Error{Kind: "MCP_ENDPOINT_FORBIDDEN"}
	}
	_, err = policy.resolve(ctx, parsed.Hostname())
	return parsed, err
}

func (policy EndpointPolicy) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	var addresses []netip.Addr
	if address, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		addresses = []netip.Addr{address.Unmap()}
	} else {
		resolver := policy.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		var err error
		addresses, err = resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, Error{Kind: "MCP_ENDPOINT_UNAVAILABLE"}
		}
	}
	if len(addresses) == 0 {
		return nil, Error{Kind: "MCP_ENDPOINT_UNAVAILABLE"}
	}
	for _, address := range addresses {
		address = address.Unmap()
		if !address.IsGlobalUnicast() || address.IsLoopback() || address.IsLinkLocalUnicast() || address == netip.MustParseAddr("100.100.100.200") || address == netip.MustParseAddr("168.63.129.16") {
			return nil, Error{Kind: "MCP_ENDPOINT_FORBIDDEN"}
		}
		for _, prefix := range policy.DeniedCIDRs {
			if prefix.Contains(address) {
				return nil, Error{Kind: "MCP_ENDPOINT_FORBIDDEN"}
			}
		}
	}
	return addresses, nil
}

func (policy EndpointPolicy) Client() *http.Client {
	// Reuse the repository's TLS roots (including its build-tag-isolated replay
	// fixture roots), but use the customer-network policy at every actual dial.
	client := (modelprovider.PublicEndpointPolicy{}).Client()
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := policy.resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
	}
	client.Timeout = 60 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return Error{Kind: "MCP_ENDPOINT_FORBIDDEN"} }
	return client
}
