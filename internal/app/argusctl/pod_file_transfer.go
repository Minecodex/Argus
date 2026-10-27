package argusctl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type podFileTransfer struct {
	Pod        string
	Container  string
	RemotePath string
	LocalPath  string
	ToPod      bool
}

// kubectl treats a Windows drive colon as a remote file separator. Resolve the
// host path locally and give kubectl a basename in a per-command directory.
// Do not change the process working directory: other commands may run at once.
func (a *App) transferPodFile(ctx context.Context, kube []string, file podFileTransfer) error {
	kube, environment, err := absoluteTransferKubeConfig(kube)
	if err != nil {
		return err
	}
	directory, args, err := podFileTransferCommand(kube, file)
	if err != nil {
		return err
	}
	_, err = a.runner.quietInDir(ctx, directory, environment, "kubectl", args...)
	return err
}

func podFileTransferCommand(kube []string, file podFileTransfer) (string, []string, error) {
	if file.Pod == "" || strings.ContainsAny(file.Pod, "/\\:\x00\r\n") ||
		file.LocalPath == "" || !strings.HasPrefix(file.RemotePath, "/") ||
		strings.ContainsAny(file.RemotePath, "\x00\r\n") {
		return "", nil, errors.New("invalid Pod file transfer")
	}
	local, err := filepath.Abs(file.LocalPath)
	if err != nil {
		return "", nil, err
	}
	base := filepath.Base(local)
	if base == "." || base == string(filepath.Separator) || strings.ContainsAny(base, ":\x00\r\n") {
		return "", nil, errors.New("Pod transfer requires a local file name")
	}
	source, destination := file.Pod+":"+file.RemotePath, base
	if file.ToPod {
		source, destination = destination, source
	}
	args := append(append([]string{}, kube...), "cp", source, destination)
	if file.Container != "" {
		args = append(args, "--container", file.Container)
	}
	return filepath.Dir(local), args, nil
}

// Preserve the caller's Kubernetes configuration when kubectl runs beside the
// local file. Relative config paths are resolved before changing command Dir;
// client-go still resolves certificate/key paths relative to each config file.
func absoluteTransferKubeConfig(kube []string) ([]string, map[string]string, error) {
	args := append([]string{}, kube...)
	for i := range args {
		if args[i] == "--kubeconfig" && i+1 < len(args) {
			absolute, err := filepath.Abs(args[i+1])
			if err != nil {
				return nil, nil, err
			}
			args[i+1] = absolute
		} else if path, found := strings.CutPrefix(args[i], "--kubeconfig="); found {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, nil, err
			}
			args[i] = "--kubeconfig=" + absolute
		}
	}
	environment := map[string]string{}
	if value, exists := os.LookupEnv("KUBECONFIG"); exists && value != "" {
		paths := filepath.SplitList(value)
		for i, path := range paths {
			if path == "" {
				continue
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, nil, err
			}
			paths[i] = absolute
		}
		environment["KUBECONFIG"] = strings.Join(paths, string(os.PathListSeparator))
	}
	return args, environment, nil
}
