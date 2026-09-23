package workspacelease

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTakeoverDrainsBeforeNewAuthority(t *testing.T) {
	lease := New("workspace", 1)
	ctx, done, err := lease.Begin(context.Background(), "workspace", 1)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- lease.Bind(context.Background(), "workspace", 1, 2, func(context.Context) error { return nil })
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("old RPC was not cancelled")
	}
	if lease.Check("workspace", 2) == nil {
		t.Fatal("new authority became active before old RPC drained")
	}
	done()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if lease.Check("workspace", 1) == nil || lease.Check("workspace", 2) != nil {
		t.Fatal("fence did not advance")
	}
	if err := lease.Release(context.Background(), "workspace", 2, nil); err != nil {
		t.Fatal(err)
	}
	if lease.Bind(context.Background(), "workspace", 1, 2, nil) == nil {
		t.Fatal("released authority was revived")
	}
	if lease.Bind(context.Background(), "workspace", 2, 3, nil) == nil {
		t.Fatal("different generation was accepted")
	}
}

func TestFailedProcessCleanupCannotActivateLease(t *testing.T) {
	lease := New("workspace", 1)
	failure := errors.New("writer still running")
	if !errors.Is(lease.Bind(context.Background(), "workspace", 1, 2, func(context.Context) error { return failure }), failure) {
		t.Fatal("cleanup failure was hidden")
	}
	if lease.Check("workspace", 1) == nil || lease.Check("workspace", 2) == nil {
		t.Fatal("authority active after failed physical cleanup")
	}
	if err := lease.Bind(context.Background(), "workspace", 1, 3, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
