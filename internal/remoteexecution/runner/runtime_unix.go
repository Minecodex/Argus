//go:build !windows

package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type bashRuntime struct {
	cmd            *exec.Cmd
	stdin, control *os.File
	reader         *bufio.Reader
	once           sync.Once
	done           chan struct{}
	closeErr       error
	version        string
}

func newRuntime(ctx context.Context, root, interpreter string, kubernetes bool) (commandRuntime, error) {
	if interpreter != "bash" {
		return nil, errors.New("REMOTE_INTERPRETER_UNAVAILABLE")
	}
	executable, err := exec.LookPath("bash")
	if err != nil {
		return nil, err
	}
	versionRaw, err := exec.CommandContext(ctx, executable, "--version").Output()
	if err != nil {
		return nil, err
	}
	version := strings.SplitN(string(versionRaw), "\n", 2)[0]
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	controlRead, controlWrite, err := os.Pipe()
	if err != nil {
		inputRead.Close()
		inputWrite.Close()
		return nil, err
	}
	cmd := exec.Command(executable, "--noprofile", "--norc", "-s")
	cmd.Env = commandEnvironment(kubernetes)
	cmd.Stdin = inputRead
	cmd.ExtraFiles = []*os.File{controlWrite}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = root
	err = cmd.Start()
	inputRead.Close()
	controlWrite.Close()
	if err != nil {
		inputWrite.Close()
		controlRead.Close()
		return nil, err
	}
	r := &bashRuntime{cmd: cmd, stdin: inputWrite, control: controlRead, reader: bufio.NewReaderSize(controlRead, 64<<10), done: make(chan struct{}), version: version}
	go func() { _ = cmd.Wait(); close(r.done) }()
	if err = ctx.Err(); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}
func (b *bashRuntime) Version() string { return b.version }
func quote(v string) string            { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }

type pipeCapture struct {
	read, write *os.File
	data        []byte
	truncated   bool
	err         error
	done        chan struct{}
}

func startCapture() (*pipeCapture, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	c := &pipeCapture{read: r, write: w, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		defer r.Close()
		buffer := make([]byte, 32<<10)
		for {
			n, e := r.Read(buffer)
			keep := min(n, maxOutput-len(c.data))
			c.data = append(c.data, buffer[:keep]...)
			c.truncated = c.truncated || n > keep
			if e != nil {
				if e != io.EOF {
					c.err = e
				}
				return
			}
		}
	}()
	return c, nil
}
func (c *pipeCapture) path() string { return fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), c.write.Fd()) }
func (c *pipeCapture) finish() error {
	c.write.Close()
	select {
	case <-c.done:
		return c.err
	case <-time.After(time.Second):
		c.read.Close()
		<-c.done
		return errors.New("REMOTE_OUTPUT_STILL_ACTIVE")
	}
}
func (b *bashRuntime) Run(ctx context.Context, script, input, output string) (result runtimeResult, runErr error) {
	result.ExitCode = -1
	check := exec.CommandContext(ctx, b.cmd.Path, "-n")
	check.Env = b.cmd.Env
	check.Stdin = strings.NewReader(script)
	if err := check.Run(); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.ExitCode = 2
		result.Stderr = []byte("Bash syntax validation failed\n")
		return result, nil
	}
	out, err := startCapture()
	if err != nil {
		return result, err
	}
	errout, err := startCapture()
	if err != nil {
		out.finish()
		return result, err
	}
	defer func() {
		e1, e2 := out.finish(), errout.finish()
		result.Stdout = out.data
		result.Stderr = errout.data
		result.Truncated = out.truncated || errout.truncated
		if runErr == nil {
			runErr = errors.Join(e1, e2)
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = b.Close() })
	defer stop()
	nonce := uuid.NewString()
	// Source is frozen inline: a resource process cannot replace a script file
	// between validation and execution. Output goes to bounded supervisor pipes.
	command := "export ARGUS_INPUT_DIR=" + quote(input) + " ARGUS_OUTPUT_DIR=" + quote(output) + "; builtin eval " + quote(script) + " </dev/null >" + quote(out.path()) + " 2>" + quote(errout.path()) + "; __argus_status=$?; builtin printf '%s\\0%s\\0%s\\0' " + quote(nonce) + " \"$__argus_status\" \"$PWD\" >&3\n"
	if _, err = b.stdin.WriteString(command); err != nil {
		return result, err
	}
	fields := make([]string, 3)
	for i := range fields {
		raw, e := readBoundedFrame(b.reader, 0, 64<<10)
		if e != nil {
			return result, e
		}
		fields[i] = string(raw[:len(raw)-1])
	}
	if fields[0] != nonce {
		return result, errors.New("REMOTE_COMPLETION_INVALID")
	}
	result.ExitCode, err = strconv.Atoi(fields[1])
	result.Directory = fields[2]
	if err == nil {
		var alive bool
		alive, err = processGroupHasChildren(b.cmd.Process.Pid)
		if err == nil && alive {
			err = errors.New("REMOTE_OUTPUT_STILL_ACTIVE")
		}
	}
	return result, err
}
func (b *bashRuntime) Close() error {
	b.once.Do(func() {
		b.closeErr = syscall.Kill(-b.cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(b.closeErr, syscall.ESRCH) {
			b.closeErr = nil
		}
		_ = b.stdin.Close()
		_ = b.control.Close()
		select {
		case <-b.done:
		case <-time.After(5 * time.Second):
			b.closeErr = errors.New("REMOTE_STOP_UNKNOWN")
		}
		// Reap the supervisor, then verify that no live descendant remains in
		// its process group. Zombies have already ceased execution.
		if b.closeErr == nil {
			b.closeErr = waitProcessGroupStopped(b.cmd.Process.Pid, 5*time.Second)
		}
	})
	return b.closeErr
}
func isLinkOrReparse(_ string, info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
