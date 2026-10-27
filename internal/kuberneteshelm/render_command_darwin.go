package kuberneteshelm

import "os/exec"

func configureRenderCommand(command *exec.Cmd) {
	// Darwin refuses to lower RLIMIT_DATA below mappings already reserved by
	// the Go runtime. Install the same hard/soft limit before exec instead.
	// The script is fixed; the executable and arguments are separate argv items.
	args := append([]string(nil), command.Args...)
	command.Path = "/bin/sh"
	command.Args = append([]string{"/bin/sh", "-c", `ulimit -Hd 131072 && ulimit -Sd 131072 && exec "$@"`, "argus-helm-renderer"}, args...)
}
