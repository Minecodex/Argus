//go:build !windows

package connector

import (
	"context"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type unixPTY struct {
	file *os.File
}

func openLocalTerminal(ctx context.Context, cols, rows uint32) (localTerminal, error) {
	command := exec.CommandContext(ctx, "/bin/sh", "-l")
	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{file: file}, nil
}

func (terminal *unixPTY) Read(value []byte) (int, error)  { return terminal.file.Read(value) }
func (terminal *unixPTY) Write(value []byte) (int, error) { return terminal.file.Write(value) }
func (terminal *unixPTY) Close() error                    { return terminal.file.Close() }
func (terminal *unixPTY) Resize(cols, rows uint32) error {
	return pty.Setsize(terminal.file, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
