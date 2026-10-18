package modelprovider

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestConnectionFailoverUsesOnlyValidatedAddressesBeforeHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	addresses := []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1")}
	attempts := []string{}
	connection, err := (PublicEndpointPolicy{}).dialValidatedAddresses(ctx, "tcp", "public.example", "443", addresses, func(attempt context.Context, _, address string) (net.Conn, error) {
		deadline, ok := attempt.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal("connection budget was extended")
		}
		attempts = append(attempts, address)
		if len(attempts) == 1 {
			return nil, errors.New("unavailable first public address")
		}
		return left, nil
	})
	if err != nil || connection != left || len(attempts) != 2 {
		t.Fatalf("public failover failed: %v %v", attempts, err)
	}
	attempts = nil
	_, err = (PublicEndpointPolicy{}).dialValidatedAddresses(ctx, "tcp", "public.example", "443", append(addresses, netip.MustParseAddr("127.0.0.1")), func(context.Context, string, string) (net.Conn, error) {
		attempts = append(attempts, "unsafe")
		return left, nil
	})
	if !errors.Is(err, ErrEndpointNotAllowed) || len(attempts) != 0 {
		t.Fatal("mixed DNS answer reached a dial")
	}
}

func TestDNSFailurePreservesSafeClassificationAndPolicyDenial(t *testing.T) {
	cause := &net.DNSError{Err: "private detail", Name: "private.example", IsTimeout: true}
	err := &endpointResolutionError{cause: cause}
	if !errors.Is(err, ErrEndpointNotAllowed) || !errors.Is(err, cause) || streamFailureType(err) != "dns_timeout" || !err.Timeout() {
		t.Fatal("DNS failure lost its cause or policy boundary")
	}
	if strings.Contains(err.Error(), "private") {
		t.Fatal("DNS error exposes endpoint details")
	}
}
