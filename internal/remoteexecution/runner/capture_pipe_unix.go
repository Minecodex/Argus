//go:build !windows

package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type capturePipe struct {
	read     io.ReadCloser
	write    *os.File
	endpoint string
	cleanup  func()
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
	// Darwin kqueue does not notify FIFO readers when the last writer closes
	// (Go issue 24164). Read the nonblocking descriptor directly rather than
	// registering it with Go's kqueue poller, which could wait forever for EOF.
	reader := &fifoCaptureReader{fd: readFD, done: make(chan struct{})}
	return capturePipe{read: reader, write: os.NewFile(uintptr(writeFD), fifo), endpoint: fifo, cleanup: cleanup}, nil
}

type fifoCaptureReader struct {
	mu     sync.Mutex
	fd     int
	closed bool
	done   chan struct{}
}

func (r *fifoCaptureReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return 0, os.ErrClosed
		}
		n, err := unix.Read(r.fd, buffer)
		r.mu.Unlock()
		if n > 0 {
			return n, nil
		}
		if err == nil {
			return 0, io.EOF
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EWOULDBLOCK) {
			return 0, err
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-r.done:
			timer.Stop()
			return 0, os.ErrClosed
		case <-timer.C:
		}
	}
}

func (r *fifoCaptureReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	// Read never blocks inside the lock. Serialize close with each syscall so
	// a recycled descriptor can never be read as though it were this FIFO.
	r.closed = true
	close(r.done)
	return unix.Close(r.fd)
}
