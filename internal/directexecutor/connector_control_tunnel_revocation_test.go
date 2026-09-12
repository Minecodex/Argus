package directexecutor

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestConnectorControlTunnelSupervisorClosesListenersWhenAuthorityChanges(t *testing.T) {
	base := db.ConnectorControlTunnel{ID: uuid.New(), EnterpriseID: uuid.New(), Status: "established",
		LeaseOwner: "executor-a", Fence: 7, Epoch: 11}
	tests := []struct {
		name   string
		change func(*db.ConnectorControlTunnel)
	}{
		{name: "removed", change: func(value *db.ConnectorControlTunnel) { value.Status = "removed" }},
		{name: "owner changed", change: func(value *db.ConnectorControlTunnel) { value.LeaseOwner = "executor-b" }},
		{name: "fence advanced", change: func(value *db.ConnectorControlTunnel) { value.Fence++ }},
		{name: "epoch advanced", change: func(value *db.ConnectorControlTunnel) { value.Epoch++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry, addresses := newRevocationTestEntry(t, base)
			current := base
			test.change(&current)
			executor := &Executor{controlTunnels: newConnectorControlTunnelSupervisor(1, 0)}
			done := make(chan struct{})
			go func() {
				executor.superviseConnectorControlTunnelWithStateReader(context.Background(), entry, 5*time.Millisecond,
					func(context.Context) (db.ConnectorControlTunnel, error) { return current, nil },
					func(context.Context, db.MarkConnectorControlTunnelDroppedParams) (int64, error) { return 0, nil })
				close(done)
			}()
			waitRevocationSupervisor(t, done)
			assertRevocationListenersClosed(t, addresses)
		})
	}
}

func TestConnectorControlTunnelSupervisorBoundsDatabaseErrors(t *testing.T) {
	entry, addresses := newRevocationTestEntry(t, db.ConnectorControlTunnel{ID: uuid.New(), EnterpriseID: uuid.New(),
		Status: "established", LeaseOwner: "executor-a", Fence: 7, Epoch: 11})
	executor := &Executor{controlTunnels: newConnectorControlTunnelSupervisor(1, 0)}
	var calls atomic.Int32
	var dropped atomic.Bool
	started := time.Now()
	done := make(chan struct{})
	go func() {
		executor.superviseConnectorControlTunnelWithStateReader(context.Background(), entry, connectorControlTunnelStateCheckInterval,
			func(context.Context) (db.ConnectorControlTunnel, error) {
				calls.Add(1)
				return db.ConnectorControlTunnel{}, errors.New("temporary database failure")
			}, func(_ context.Context, input db.MarkConnectorControlTunnelDroppedParams) (int64, error) {
				if input.Fence != entry.tunnel.Fence || input.LeaseOwner != entry.tunnel.LeaseOwner {
					t.Errorf("drop used a different authority: %+v", input)
				}
				dropped.Store(true)
				return 1, nil
			})
		close(done)
	}()
	waitRevocationSupervisorWithin(t, done, 4*time.Second)
	if got := calls.Load(); got != connectorControlTunnelStateErrorLimit {
		t.Fatalf("state reader calls = %d, want %d", got, connectorControlTunnelStateErrorLimit)
	}
	assertRevocationListenersClosed(t, addresses)
	if elapsed := time.Since(started); elapsed < 2500*time.Millisecond || elapsed > 4*time.Second {
		t.Fatalf("production state checks closed after %s", elapsed)
	}
	for deadline := time.Now().Add(time.Second); !dropped.Load() && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if !dropped.Load() {
		t.Fatal("database tunnel owner was not cleared after local close")
	}
}

func TestConnectorControlTunnelSupervisorRecoversFromTransientDatabaseError(t *testing.T) {
	state := db.ConnectorControlTunnel{ID: uuid.New(), EnterpriseID: uuid.New(), Status: "established",
		LeaseOwner: "executor-a", Fence: 7, Epoch: 11}
	entry, addresses := newRevocationTestEntry(t, state)
	executor := &Executor{controlTunnels: newConnectorControlTunnelSupervisor(1, 0)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checked := make(chan struct{}, 2)
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		executor.superviseConnectorControlTunnelWithStateReader(ctx, entry, 5*time.Millisecond,
			func(context.Context) (db.ConnectorControlTunnel, error) {
				call := calls.Add(1)
				select {
				case checked <- struct{}{}:
				default:
				}
				if call == 1 {
					return db.ConnectorControlTunnel{}, errors.New("temporary database failure")
				}
				return state, nil
			}, func(context.Context, db.MarkConnectorControlTunnelDroppedParams) (int64, error) { return 0, nil })
		close(done)
	}()
	for range 2 {
		select {
		case <-checked:
		case <-time.After(time.Second):
			t.Fatal("state reader was not called twice")
		}
	}
	for _, address := range addresses {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("listener closed after recoverable database error: %v", err)
		}
		_ = connection.Close()
	}
	cancel()
	waitRevocationSupervisor(t, done)
}

func TestConnectorControlTunnelSupervisorRetriesDropAfterDatabaseRecovery(t *testing.T) {
	state := db.ConnectorControlTunnel{ID: uuid.New(), EnterpriseID: uuid.New(), Status: "established",
		LeaseOwner: "executor-a", Fence: 7, Epoch: 11,
		LeaseExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(10 * time.Second), Valid: true}}
	entry, addresses := newRevocationTestEntry(t, state)
	executor := &Executor{controlTunnels: newConnectorControlTunnelSupervisor(1, 0)}
	var dropCalls atomic.Int32
	dropped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		executor.superviseConnectorControlTunnelWithStateReader(context.Background(), entry, 5*time.Millisecond,
			func(context.Context) (db.ConnectorControlTunnel, error) {
				return db.ConnectorControlTunnel{}, errors.New("database unavailable")
			}, func(_ context.Context, input db.MarkConnectorControlTunnelDroppedParams) (int64, error) {
				if dropCalls.Add(1) == 1 {
					return 0, errors.New("database still unavailable")
				}
				if input.ID != state.ID || input.EnterpriseID != state.EnterpriseID || input.Fence != state.Fence || input.LeaseOwner != state.LeaseOwner {
					t.Errorf("drop lost the old authority fence: %+v", input)
				}
				state.Status, state.LeaseOwner = input.Status, ""
				close(dropped)
				return 1, nil
			})
		close(done)
	}()
	waitRevocationSupervisor(t, done)
	assertRevocationListenersClosed(t, addresses)
	select {
	case <-dropped:
	case <-time.After(2 * time.Second):
		t.Fatal("tunnel drop was not retried after database recovery")
	}
	if state.Status != "degraded" || state.LeaseOwner != "" || dropCalls.Load() != 2 {
		t.Fatalf("recovered authoritative state = %s owner %q calls %d", state.Status, state.LeaseOwner, dropCalls.Load())
	}
}

func TestConnectorControlTunnelSupervisorDropCannotOverwriteNewOwner(t *testing.T) {
	old := db.ConnectorControlTunnel{ID: uuid.New(), EnterpriseID: uuid.New(), Status: "established",
		LeaseOwner: "executor-a", Fence: 7, Epoch: 11,
		LeaseExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(10 * time.Second), Valid: true}}
	current := old
	current.LeaseOwner, current.Fence, current.Epoch = "executor-b", 8, 12
	entry, _ := newRevocationTestEntry(t, old)
	executor := &Executor{controlTunnels: newConnectorControlTunnelSupervisor(1, 0)}
	var dropCalls atomic.Int32
	dropFinished := make(chan struct{})
	done := make(chan struct{})
	go func() {
		executor.superviseConnectorControlTunnelWithStateReader(context.Background(), entry, 5*time.Millisecond,
			func(context.Context) (db.ConnectorControlTunnel, error) {
				return db.ConnectorControlTunnel{}, errors.New("database unavailable")
			}, func(_ context.Context, input db.MarkConnectorControlTunnelDroppedParams) (int64, error) {
				dropCalls.Add(1)
				if input.Fence == current.Fence && input.LeaseOwner == current.LeaseOwner {
					current.Status, current.LeaseOwner = input.Status, ""
					return 1, nil
				}
				close(dropFinished)
				return 0, nil
			})
		close(done)
	}()
	waitRevocationSupervisor(t, done)
	select {
	case <-dropFinished:
	case <-time.After(time.Second):
		t.Fatal("stale drop did not finish")
	}
	if current.Status != "established" || current.LeaseOwner != "executor-b" || current.Fence != 8 || dropCalls.Load() != 1 {
		t.Fatalf("stale drop overwrote new authority: %+v calls=%d", current, dropCalls.Load())
	}
}

func TestConnectorControlTunnelStateReadHasDeadline(t *testing.T) {
	state := db.ConnectorControlTunnel{ID: uuid.New(), EnterpriseID: uuid.New(), Status: "established",
		LeaseOwner: "executor-a", Fence: 7, Epoch: 11}
	entry, _ := newRevocationTestEntry(t, state)
	executor := &Executor{controlTunnels: newConnectorControlTunnelSupervisor(1, 0)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	readRecovered := make(chan struct{})
	done := make(chan struct{})
	go func() {
		executor.superviseConnectorControlTunnelWithStateReader(ctx, entry, 5*time.Millisecond,
			func(readCtx context.Context) (db.ConnectorControlTunnel, error) {
				if calls.Add(1) == 1 {
					<-readCtx.Done()
					return db.ConnectorControlTunnel{}, readCtx.Err()
				}
				close(readRecovered)
				return state, nil
			}, func(context.Context, db.MarkConnectorControlTunnelDroppedParams) (int64, error) { return 0, nil })
		close(done)
	}()
	select {
	case <-readRecovered:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked state read had no bounded deadline")
	}
	cancel()
	waitRevocationSupervisor(t, done)
}

func newRevocationTestEntry(t *testing.T, tunnel db.ConnectorControlTunnel) (*connectorControlTunnelEntry, []string) {
	t.Helper()
	enroll, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = enroll.Close()
		t.Fatal(err)
	}
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = enroll.Close()
		_ = gateway.Close()
	})
	return &connectorControlTunnelEntry{tunnel: tunnel, enroll: enroll, gateway: gateway, cancel: cancel},
		[]string{enroll.Addr().String(), gateway.Addr().String()}
}

func waitRevocationSupervisor(t *testing.T, done <-chan struct{}) {
	t.Helper()
	waitRevocationSupervisorWithin(t, done, time.Second)
}

func waitRevocationSupervisorWithin(t *testing.T, done <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("control tunnel supervisor did not stop")
	}
}

func assertRevocationListenersClosed(t *testing.T, addresses []string) {
	t.Helper()
	for _, address := range addresses {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			t.Fatalf("listener %s still accepted connections", address)
		}
	}
}
