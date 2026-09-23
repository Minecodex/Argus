// Package workspacelease fences active RPCs and drains the previous operation
// before a persistent compute/file workload accepts its next lease.
package workspacelease

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrAuthority = errors.New("workspace lease authority mismatch")

type Lease struct {
	mu                sync.Mutex
	transition        sync.Mutex
	id                string
	generation, fence int64
	active            bool
	next              uint64
	calls             map[uint64]context.CancelFunc
	drained           chan struct{}
	lastUsed          time.Time
}

func New(id string, generation int64) *Lease {
	return &Lease{id: id, generation: generation, fence: generation, active: true, calls: map[uint64]context.CancelFunc{}, lastUsed: time.Now()}
}

func (l *Lease) IdleFor(duration time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls) == 0 && time.Since(l.lastUsed) >= duration
}

func (l *Lease) Check(id string, fence int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.active || id != l.id || fence != l.fence || fence <= 0 {
		return ErrAuthority
	}
	return nil
}

func (l *Lease) Begin(ctx context.Context, id string, fence int64) (context.Context, func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.active || id != l.id || fence != l.fence || fence <= 0 {
		return nil, nil, ErrAuthority
	}
	ctx, cancel := context.WithCancel(ctx)
	if len(l.calls) == 0 {
		l.drained = make(chan struct{})
	}
	l.next++
	index := l.next
	l.calls[index] = cancel
	l.lastUsed = time.Now()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			cancel()
			l.mu.Lock()
			delete(l.calls, index)
			if len(l.calls) == 0 {
				close(l.drained)
			}
			l.lastUsed = time.Now()
			l.mu.Unlock()
		})
	}, nil
}

func (l *Lease) Bind(ctx context.Context, id string, generation, fence int64, cleanup func(context.Context) error) error {
	l.transition.Lock()
	defer l.transition.Unlock()
	l.mu.Lock()
	if id != l.id || generation != l.generation || fence < l.fence || fence < generation || fence <= 0 || fence == l.fence && !l.active {
		l.mu.Unlock()
		return ErrAuthority
	}
	if fence == l.fence {
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()
	if err := l.drain(ctx, cleanup); err != nil {
		return err
	}
	l.mu.Lock()
	l.fence = fence
	l.active = true
	l.lastUsed = time.Now()
	l.mu.Unlock()
	return nil
}

func (l *Lease) Release(ctx context.Context, id string, fence int64, cleanup func(context.Context) error) error {
	l.transition.Lock()
	defer l.transition.Unlock()
	l.mu.Lock()
	if id != l.id || fence != l.fence || fence <= 0 {
		l.mu.Unlock()
		return ErrAuthority
	}
	l.mu.Unlock()
	return l.drain(ctx, cleanup)
}

func (l *Lease) drain(ctx context.Context, cleanup func(context.Context) error) error {
	l.mu.Lock()
	l.active = false
	l.lastUsed = time.Now()
	for _, cancel := range l.calls {
		cancel()
	}
	done := l.drained
	if len(l.calls) == 0 {
		done = nil
	}
	l.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if cleanup != nil {
		return cleanup(ctx)
	}
	return nil
}
