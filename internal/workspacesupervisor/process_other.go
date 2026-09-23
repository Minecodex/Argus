//go:build !linux

package workspacesupervisor

import (
	"context"
	"errors"
	"os/exec"
)

func platformReady() error                             { return errors.New("offline supervisor requires Linux") }
func configureChild(*exec.Cmd, uint32)                 {}
func Child(int, string) error                          { return platformReady() }
func sweepUserProcesses(context.Context, uint32) error { return platformReady() }
