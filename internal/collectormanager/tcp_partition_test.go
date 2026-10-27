package collectormanager

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// Transparent TCP partition for the Collector's real TLS Kubernetes API path.
// The fixture never decrypts, records, or changes Kubernetes requests.
type partitionedAPI struct {
	listener    net.Listener
	target      string
	mu          sync.Mutex
	paused      bool
	connections map[net.Conn]bool
}

func newPartitionedAPI(t *testing.T, target string) *partitionedAPI {
	t.Helper()
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &partitionedAPI{listener: listener, target: target, connections: map[net.Conn]bool{}}
	t.Cleanup(func() { _ = p.listener.Close(); p.partition(true) })
	go func() {
		for {
			client, err := p.listener.Accept()
			if err != nil {
				return
			}
			go p.forward(client)
		}
	}()
	return p
}
func (p *partitionedAPI) partition(paused bool) {
	p.mu.Lock()
	p.paused = paused
	connections := []net.Conn{}
	if paused {
		for connection := range p.connections {
			connections = append(connections, connection)
		}
	}
	p.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
}
func (p *partitionedAPI) forward(client net.Conn) {
	defer client.Close()
	p.mu.Lock()
	paused := p.paused
	p.mu.Unlock()
	if paused {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", p.target)
	if err != nil {
		return
	}
	defer upstream.Close()
	p.mu.Lock()
	if p.paused {
		p.mu.Unlock()
		return
	}
	p.connections[client], p.connections[upstream] = true, true
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.connections, client); delete(p.connections, upstream); p.mu.Unlock() }()
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close(); close(done) }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	_ = upstream.Close()
	<-done
}
