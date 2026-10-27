package kuberneteshelm

import "os/exec"

func configureRenderCommand(command *exec.Cmd) {
	// taskpolicy uses POSIX_SPAWN_SETEXEC and installs fatal active/inactive
	// memory limits before the renderer starts. RLIMIT_DATA also counts shared
	// writable mappings on Darwin and cannot enforce this footprint budget.
	args := append([]string(nil), command.Args...)
	command.Path = "/usr/bin/taskpolicy"
	command.Args = append([]string{command.Path, "-m", "128"}, args...)
}
