package modelprovider

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type droppingTLSListener struct {
	net.Listener
	remaining atomic.Int32
	accepted  atomic.Int32
}

func (l *droppingTLSListener) Accept() (net.Conn, error) {
	for {
		c, e := l.Listener.Accept()
		if e != nil {
			return nil, e
		}
		l.accepted.Add(1)
		if l.remaining.Add(-1) >= 0 {
			c.Close()
			continue
		}
		return c, nil
	}
}
func retryServer(t *testing.T, n int32, h http.Handler) (*httptest.Server, *droppingTLSListener) {
	t.Helper()
	s := httptest.NewUnstartedServer(h)
	l := &droppingTLSListener{Listener: s.Listener}
	l.remaining.Store(n)
	s.Listener = l
	s.StartTLS()
	t.Cleanup(s.Close)
	return s, l
}
func TestTLSRetryBeforeHTTPWritesOnlyOnePOST(t *testing.T) {
	var calls atomic.Int32
	s, l := retryServer(t, 2, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		data, _ := io.ReadAll(r.Body)
		if string(data) != "original payload" {
			t.Error("body changed")
		}
		w.Write([]byte("OK"))
	}))
	request, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader("original payload"))
	response, e := doModelRequest(s.Client(), request)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if l.accepted.Load() != 3 || calls.Load() != 1 {
		t.Fatal("wrong transport/HTTP count", l.accepted.Load(), calls.Load())
	}
}
func TestTLSRetryStopsAfterThreeHandshakes(t *testing.T) {
	s, l := retryServer(t, 5, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unexpected HTTP request") }))
	r, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader("body"))
	_, e := doModelRequest(s.Client(), r)
	if e == nil || l.accepted.Load() != 3 {
		t.Fatal("unbounded TLS retry", e, l.accepted.Load())
	}
}
func TestPostWriteEOFNeverReplays(t *testing.T) {
	var calls atomic.Int32
	s, _ := retryServer(t, 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	r, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader("already dispatched"))
	_, e := doModelRequest(s.Client(), r)
	if e == nil || calls.Load() != 1 {
		t.Fatal("replayed an accepted request", e, calls.Load())
	}
}
func TestCertificateRejectionNeverRetries(t *testing.T) {
	s, l := retryServer(t, 0, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("invalid certificate accepted") }))
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	defer tr.CloseIdleConnections()
	r, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader("body"))
	_, e := doModelRequest(&http.Client{Transport: tr}, r)
	if e == nil || l.accepted.Load() != 1 {
		t.Fatal("retried invalid certificate", e, l.accepted.Load())
	}
}

type retryRoundTripper struct{ calls atomic.Int32 }

func (r *retryRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return nil, io.EOF
}
func TestCustomTransportCannotClaimPreDispatchRetry(t *testing.T) {
	tr := &retryRoundTripper{}
	r, _ := http.NewRequest(http.MethodPost, "https://example.test", strings.NewReader("body"))
	_, e := doModelRequest(&http.Client{Transport: tr}, r)
	if e == nil || tr.calls.Load() != 1 {
		t.Fatal(e, tr.calls.Load())
	}
}
func TestCancelledTLSRetryDoesNotContinue(t *testing.T) {
	s, l := retryServer(t, 5, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unexpected HTTP request") }))
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, strings.NewReader("body"))
	_, e := doModelRequest(s.Client(), r)
	if !errors.Is(e, context.DeadlineExceeded) || l.accepted.Load() != 1 {
		t.Fatal(e, l.accepted.Load())
	}
}

func TestNonReplayableBodyDoesNotRetryTLSFailure(t *testing.T) {
	s, l := retryServer(t, 2, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unexpected HTTP request") }))
	r, _ := http.NewRequest(http.MethodPost, s.URL, io.NopCloser(strings.NewReader("body")))
	_, e := doModelRequest(s.Client(), r)
	if e == nil || l.accepted.Load() != 1 {
		t.Fatal("retried non-replayable body", e, l.accepted.Load())
	}
}
func TestProviderErrorResponseNeverRetries(t *testing.T) {
	var calls atomic.Int32
	s, _ := retryServer(t, 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	r, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader("body"))
	response, e := doModelRequest(s.Client(), r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatal("retried provider response", calls.Load())
	}
}
