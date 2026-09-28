package opensandbox

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type timeoutTransport func(*http.Request) (*http.Response, error)

func (f timeoutTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCreationCoversServerReadinessWithoutExtendingOtherRequests(t *testing.T) {
	client, err := NewClient("https://sandbox.example.test", "")
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = timeoutTransport(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("request has no deadline")
		}
		remaining := time.Until(deadline)
		if r.Method == http.MethodPost {
			if remaining < 60*time.Second || remaining > 90*time.Second {
				t.Fatalf("creation cannot cover upstream readiness: %v", remaining)
			}
		} else if remaining > 30*time.Second {
			t.Fatalf("ordinary request timeout extended: %v", remaining)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"test"}`))}, nil
	})
	if _, err = client.Create(t.Context(), CreateRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Get(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	if client.http.Timeout != 30*time.Second {
		t.Fatal("shared client was mutated")
	}
}

func TestCreationStillHonorsShortCallerDeadlineAndDoesNotRetry(t *testing.T) {
	client, err := NewClient("https://sandbox.example.test", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	client.http.Transport = timeoutTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	if _, err = client.Create(ctx, CreateRequest{}); err == nil || calls != 1 {
		t.Fatalf("cancelled create retried or ignored deadline: %v/%d", err, calls)
	}
}
