//go:build !windows

package kuberneteshelm

import (
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

type unixRenderLimiter struct{}

func newRenderLimiter() (renderLimiter, error)     { return unixRenderLimiter{}, nil }
func (unixRenderLimiter) Attach(*os.Process) error { return nil }
func (unixRenderLimiter) Close() error             { return nil }
func configureRenderCommand(*exec.Cmd)             {}
func limitRenderSelf() error {
	for key, value := range map[int]uint64{unix.RLIMIT_CPU: 5, unix.RLIMIT_DATA: 128 << 20, unix.RLIMIT_FSIZE: 1 << 20} {
		if e := unix.Setrlimit(key, &unix.Rlimit{Cur: value, Max: value}); e != nil {
			return e
		}
	}
	return nil
}
