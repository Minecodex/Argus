//go:build !windows

package runner

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPrivateFIFOCaptureStreamsBeforeWriterAndCleansUp(t *testing.T) {
	pipe, err := openNamedCapturePipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.cleanup()
	defer pipe.read.Close()
	defer pipe.write.Close()
	ready := make(chan []byte, 1)
	failures := make(chan error, 1)
	go func() {
		data, err := io.ReadAll(pipe.read)
		if err != nil {
			failures <- err
			return
		}
		ready <- data
	}()
	// No child has opened the FIFO yet. The supervisor writer keeps it alive.
	select {
	case <-ready:
		t.Fatal("early EOF before Bash redirection")
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(20 * time.Millisecond):
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-c", "head -c 1048576 /dev/zero >"+quote(pipe.endpoint))
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("FIFO redirection: %v %s", err, data)
	}
	if err := pipe.write.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-ready:
		if len(data) != 1<<20 || !bytes.Equal(data, make([]byte, 1<<20)) {
			t.Fatal("streamed bytes changed")
		}
	case err := <-failures:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("FIFO did not reach EOF after all writers closed")
	}
	pipe.cleanup()
	if _, err := os.Stat(pipe.endpoint); !os.IsNotExist(err) {
		t.Fatal("private FIFO was not removed")
	}
}
