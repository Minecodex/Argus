//go:build linux

package workspacesupervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func platformReady() error {
	if os.Geteuid() != ManagerUID {
		return errors.New("supervisor must run as its dedicated non-root UID")
	}
	capabilities := [2]unix.CapUserData{}
	if err := unix.Capget(&unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}, &capabilities[0]); err != nil {
		return err
	}
	const required = uint32(1<<unix.CAP_SETUID | 1<<unix.CAP_SETGID | 1<<unix.CAP_KILL)
	if capabilities[0].Effective != required || capabilities[1].Effective != 0 {
		return errors.New("supervisor requires only SETUID, SETGID and KILL capabilities")
	}
	fd, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		return fmt.Errorf("pidfd process fencing unavailable: %w", err)
	}
	_ = unix.Close(fd)
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return err
	}
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
}

func configureChild(cmd *exec.Cmd, uid uint32) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{uid}}}
}

// Child is entered only after exec.Cmd has dropped UID/GID and all inherited
// capabilities. This path can never turn a user process into the manager.
func Child(limit int, command string) error {
	if os.Geteuid() < 100000 || limit < 16 || limit > 4096 {
		return errors.New("invalid command UID or process limit")
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return err
	}
	capabilities := [2]unix.CapUserData{}
	if err := unix.Capset(&unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}, &capabilities[0]); err != nil {
		return err
	}
	if err := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: uint64(limit), Max: uint64(limit)}); err != nil {
		return err
	}
	return unix.Exec("/bin/bash", []string{"bash", "--noprofile", "--norc", "-c", command}, os.Environ())
}

func sweepUserProcesses(ctx context.Context, uid uint32) error {
	for {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return err
		}
		found := false
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}
			fd, err := unix.PidfdOpen(pid, 0)
			if errors.Is(err, unix.ESRCH) {
				continue
			}
			if err != nil {
				return err
			}
			data, err := os.ReadFile("/proc/" + entry.Name() + "/status")
			if os.IsNotExist(err) {
				_ = unix.Close(fd)
				continue
			}
			if err != nil {
				_ = unix.Close(fd)
				return err
			}
			matches, zombie, parent := false, false, 0
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				switch fields[0] {
				case "Uid:":
					for _, raw := range fields[1:] {
						value, _ := strconv.ParseUint(raw, 10, 32)
						matches = matches || uint32(value) == uid
					}
				case "State:":
					zombie = fields[1] == "Z"
				case "PPid:":
					parent, _ = strconv.Atoi(fields[1])
				}
			}
			if matches {
				found = true
				if zombie && parent == os.Getpid() {
					var wait unix.WaitStatus
					_, _ = unix.Wait4(pid, &wait, unix.WNOHANG, nil)
				} else if !zombie {
					err = unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0)
					if err != nil && !errors.Is(err, unix.ESRCH) {
						_ = unix.Close(fd)
						return err
					}
				}
			}
			_ = unix.Close(fd)
		}
		if !found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
