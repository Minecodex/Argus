//go:build !windows

package collectormanager_test

import "os/exec"

func hideNativeSDKWindow(*exec.Cmd) {}
