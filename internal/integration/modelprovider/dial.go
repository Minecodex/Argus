package modelprovider

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"
)

type endpointResolutionError struct{ cause error }

func (e *endpointResolutionError) Error() string        { return "model endpoint DNS resolution failed" }
func (e *endpointResolutionError) Unwrap() error        { return e.cause }
func (e *endpointResolutionError) Is(target error) bool { return target == ErrEndpointNotAllowed }
func (e *endpointResolutionError) Timeout() bool {
	var value net.Error
	return errors.As(e.cause, &value) && value.Timeout()
}
func (e *endpointResolutionError) Temporary() bool {
	var value net.Error
	return errors.As(e.cause, &value) && value.Temporary()
}

// Only TCP connection establishment is retried, before an HTTP request or its
// credentials can be sent. Every resolved address is checked before any dial;
// a mixed public/private answer cannot fall through to the public candidate.
func (policy PublicEndpointPolicy) dialValidatedAddresses(ctx context.Context, network, host, port string, addresses []netip.Addr, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	if len(addresses) == 0 {
		return nil, ErrEndpointNotAllowed
	}
	for _, address := range addresses {
		if !policy.allowedHost(host, address) {
			return nil, ErrEndpointNotAllowed
		}
	}
	var last error
	for index, address := range addresses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attempt := ctx
		cancel := func() {}
		if deadline, ok := ctx.Deadline(); ok && len(addresses)-index > 1 {
			remaining := time.Until(deadline) / time.Duration(len(addresses)-index)
			if remaining > 10*time.Second {
				remaining = 10 * time.Second
			}
			attempt, cancel = context.WithTimeout(ctx, remaining)
		}
		connection, err := dial(attempt, network, net.JoinHostPort(address.String(), port))
		cancel()
		if err == nil {
			return connection, nil
		}
		if connection != nil {
			_ = connection.Close()
		}
		last = err
	}
	return nil, last
}
