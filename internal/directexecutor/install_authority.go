package directexecutor

import (
	"context"
	"time"
)

// Check operation ownership separately from the thirty-second lease renewal.
// Revoking a pending installation must promptly cancel its SSH context without
// waiting for the remote command or the original installation deadline.
func watchInstallAuthority(ctx context.Context, cancel context.CancelFunc, done <-chan struct{}, read func(context.Context) (bool, error)) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	errors := 0
	for {
		readCtx, cancelRead := context.WithTimeout(ctx, 2*time.Second)
		active, err := read(readCtx)
		cancelRead()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			errors++
		} else {
			errors = 0
		}
		if err == nil && !active || errors >= 3 {
			cancel()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
		}
	}
}
