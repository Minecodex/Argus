package runtimewire

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func TestProxyRequestsCannotOutliveOrBorrowAnotherApprovedBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	local, peer := net.Pipe()
	defer peer.Close()
	_ = peer.SetDeadline(time.Now().Add(5 * time.Second))
	started, cancelled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := NewClient(local, local, nil, func(ctx context.Context, _ ProxyRequest) ProxyResponse {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		close(cancelled)
		return ProxyResponse{Status: http.StatusServiceUnavailable}
	})
	defer client.Close()
	decoder, encoder := json.NewDecoder(peer), json.NewEncoder(peer)
	read := func() Message {
		t.Helper()
		var message Message
		if err := decoder.Decode(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}
	write := func(message Message) {
		t.Helper()
		if err := encoder.Encode(message); err != nil {
			t.Fatal(err)
		}
	}
	start := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := client.Call(ctx, "remote_execution_batch", &connectorv1.RemoteExecutionBatch{})
			done <- err
		}()
		return done
	}
	firstDone := start()
	first := read()
	write(Message{ID: "proxy-first", ProxyRequest: &ProxyRequest{BatchID: first.ID, Method: "GET", URI: "/api"}})
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("approved proxy did not start")
	}
	write(Message{ID: first.ID, Payload: json.RawMessage(`{}`)})
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("proxy outlived its batch")
	}
	if reply := read(); reply.ID != "proxy-first" {
		t.Fatalf("wrong proxy result: %#v", reply)
	}

	secondDone := start()
	second := read()
	write(Message{ID: "proxy-stale", ProxyRequest: &ProxyRequest{BatchID: first.ID, Method: "DELETE", URI: "/api"}})
	stale := read()
	if stale.ProxyResponse == nil || stale.ProxyResponse.Status != http.StatusForbidden || calls.Load() != 1 {
		t.Fatal("a stale batch borrowed the new approval")
	}
	write(Message{ID: second.ID, Payload: json.RawMessage(`{}`)})
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}
