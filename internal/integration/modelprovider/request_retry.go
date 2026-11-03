package modelprovider

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

// doModelRequest retries only a failed TLS handshake before any HTTP connection
// was handed to the request. It never replays a possibly dispatched model call.
func doModelRequest(client *http.Client, request *http.Request) (*http.Response, error) {
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if _, ok := transport.(*http.Transport); !ok || request.Body != nil && request.Body != http.NoBody && request.GetBody == nil {
		return client.Do(request)
	}
	if request.Body != nil {
		defer request.Body.Close()
	}
	for attempt := 0; ; attempt++ {
		var handshakeFailed, connected, wroteHeaders atomic.Bool
		trace := &httptrace.ClientTrace{
			TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
				if err != nil {
					handshakeFailed.Store(true)
				}
			},
			GotConn:      func(httptrace.GotConnInfo) { connected.Store(true) },
			WroteHeaders: func() { wroteHeaders.Store(true) },
		}
		current := request.Clone(httptrace.WithClientTrace(request.Context(), trace))
		if request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return nil, err
			}
			current.Body = body
		}
		response, err := client.Do(current)
		if err == nil || attempt >= 2 || request.Context().Err() != nil || !handshakeFailed.Load() || connected.Load() || wroteHeaders.Load() || !transientTLSHandshake(err) {
			return response, err
		}
		slog.Warn("retrying model TLS handshake before HTTP dispatch", "attempt", attempt+2, "maximum_attempts", 3)
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-request.Context().Done():
			timer.Stop()
			return nil, request.Context().Err()
		case <-timer.C:
		}
	}
}
func transientTLSHandshake(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	// Winsock uses native WSA codes rather than the portable syscall aliases.
	var socket syscall.Errno
	if errors.As(err, &socket) && (socket == syscall.ECONNABORTED || runtime.GOOS == "windows" && (socket == 10053 || socket == 10054)) {
		return true
	}
	var network net.Error
	return errors.As(err, &network) && network.Timeout()
}
