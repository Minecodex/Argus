//go:build !linux && !windows && !darwin

package runner

import (
	"errors"
	"syscall"
	"time"
)

func waitProcessGroupStopped(group int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if errors.Is(syscall.Kill(-group, 0), syscall.ESRCH) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("REMOTE_STOP_UNKNOWN")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func processGroupHasChildren(_ int) (bool, error) {
	return false, errors.New("REMOTE_PLATFORM_UNAVAILABLE")
}
