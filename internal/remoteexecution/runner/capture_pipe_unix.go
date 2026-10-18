//go:build !windows

package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"
)

type capturePipe struct {
	read, write *os.File
	endpoint    string
	cleanup     func()
}

func openCapturePipe() (capturePipe, error) {
	if runtime.GOOS != "linux" {
		return openNamedCapturePipe()
	}
	read, write, err := os.Pipe()
	if err != nil {
		return capturePipe{}, err
	}
	return capturePipe{read: read, write: write, endpoint: fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), write.Fd())}, nil
}

// A private FIFO exposes the same bounded streaming channel on Unix systems
// without procfs. Keep a parent writer open until collection finishes, so the
// reader cannot observe an early EOF before Bash opens its redirection.
func openNamedCapturePipe() (capturePipe, error) {
	directory, err := os.MkdirTemp("", "argus-output-")
	if err != nil {
		return capturePipe{}, err
	}
	fifo := filepath.Join(directory, "stream")
	cleanup := func() { _ = os.Remove(fifo); _ = os.Remove(directory) }
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		cleanup()
		return capturePipe{}, err
	}
	readFD, err := unix.Open(fifo, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		cleanup()
		return capturePipe{}, err
	}
	writeFD, err := unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(readFD)
		cleanup()
		return capturePipe{}, err
	}
	// NewFile recognizes the nonblocking descriptors and binds them to Go's
	// runtime poller; do not switch them back to blocking after registration.
	return capturePipe{read: os.NewFile(uintptr(readFD), fifo), write: os.NewFile(uintptr(writeFD), fifo), endpoint: fifo, cleanup: cleanup}, nil
}
