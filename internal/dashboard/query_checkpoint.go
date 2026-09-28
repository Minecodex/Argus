//go:build !m4e2e

package dashboard

import "context"

// Deployment fault checkpoints are compiled out of ordinary builds.
func (w queryWork) checkpoint(context.Context, string) error { return nil }
func withQueryCheckpoint(ctx context.Context, _ func(context.Context, string) error) context.Context {
	return ctx
}
func afterQueryTarget(context.Context) error { return nil }
