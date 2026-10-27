//go:build !windows

package kuberneteshelm

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

type unixRenderLimiter struct{}

func newRenderLimiter() (renderLimiter, error)     { return unixRenderLimiter{}, nil }
func (unixRenderLimiter) Attach(*os.Process) error { return nil }
func (unixRenderLimiter) Close() error             { return nil }
func limitRenderSelf() error {
	limits := map[int]uint64{unix.RLIMIT_CPU: 5, unix.RLIMIT_FSIZE: 1 << 20}
	if runtime.GOOS != "darwin" {
		limits[unix.RLIMIT_DATA] = 128 << 20
	}
	for key, value := range limits {
		if e := unix.Setrlimit(key, &unix.Rlimit{Cur: value, Max: value}); e != nil {
			return fmt.Errorf("set renderer resource %d to %d: %w", key, value, e)
		}
	}
	return nil
}
