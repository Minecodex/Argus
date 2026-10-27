//go:build windows

package collectormanager_test

import (
	"os/exec"
	"syscall"
)

func hideNativeSDKWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
