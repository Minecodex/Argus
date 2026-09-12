package directexecutor

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallAuthorityRevocationClosesExecutionConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	stop := context.AfterFunc(ctx, func() { _ = local.Close() })
	defer stop()
	closed := make(chan struct{})
	go func() { _, _ = remote.Read(make([]byte, 1)); close(closed) }()
	var active atomic.Bool
	active.Store(true)
	checked := make(chan struct{}, 1)
	done := make(chan struct{})
	defer close(done)
	go watchInstallAuthority(ctx, cancel, done, func(context.Context) (bool, error) {
		select {
		case checked <- struct{}{}:
		default:
		}
		return active.Load(), nil
	})
	select {
	case <-checked:
	case <-time.After(time.Second):
		t.Fatal("authority was not checked before the first renewal")
	}
	active.Store(false)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("revoked installation kept its connection open")
	}
}

func TestInstallAuthorityReadIsBoundedAndFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go watchInstallAuthority(ctx, cancel, done, func(readCtx context.Context) (bool, error) {
		deadline, ok := readCtx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Error("authority read has no bounded deadline")
		}
		return false, errors.New("database unavailable")
	})
	select {
	case <-ctx.Done():
	case <-time.After(4 * time.Second):
		t.Fatal("unverifiable installation continued")
	}
}

func TestInstallAuthorityMonitorStopsWhenAttemptCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	checked := make(chan struct{}, 1)
	finished := make(chan struct{})
	go func() {
		watchInstallAuthority(ctx, cancel, done, func(context.Context) (bool, error) { checked <- struct{}{}; return true, nil })
		close(finished)
	}()
	<-checked
	close(done)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("completed attempt retained an authority monitor")
	}
	if ctx.Err() != nil {
		t.Fatal("normal completion was treated as revocation")
	}
}
