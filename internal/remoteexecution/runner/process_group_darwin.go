//go:build darwin

package runner

import (
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

// SZOMB in Apple's bsd/sys/proc.h: the process has ceased execution and only
// awaits collection by its parent. It cannot keep an approved batch running.
const darwinZombieProcess = 5

func processGroupHasChildren(group int) (bool, error) { return processGroupActive(group, group) }
func processGroupActive(group, ignore int) (bool, error) {
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", group)
	if err != nil {
		return false, errors.New("REMOTE_STOP_UNKNOWN")
	}
	for _, process := range processes {
		if int(process.Proc.P_pid) != ignore && process.Proc.P_stat != darwinZombieProcess {
			return true, nil
		}
	}
	return false, nil
}
func waitProcessGroupStopped(group int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		alive, err := processGroupActive(group, 0)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("REMOTE_STOP_UNKNOWN")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
