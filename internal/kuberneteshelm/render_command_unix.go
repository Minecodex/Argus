//go:build !windows && !darwin

package kuberneteshelm

import "os/exec"

func configureRenderCommand(*exec.Cmd) {}
