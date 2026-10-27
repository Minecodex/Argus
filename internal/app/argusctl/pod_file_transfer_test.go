package argusctl

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPodFileTransferKeepsLocalDriveOutOfKubectlFileSpec(t *testing.T) {
	kube := []string{"--context", "owned", "--namespace", "backup"}
	original := append([]string{}, kube...)
	local := filepath.Join(t.TempDir(), "directory with spaces", "private snapshot.bin")
	for _, upload := range []bool{false, true} {
		directory, args, err := podFileTransferCommand(kube, podFileTransfer{
			Pod: "argus-openbao-0", Container: openBaoMaintenanceContainer,
			RemotePath: "/tmp/snapshot.bin", LocalPath: local, ToPod: upload,
		})
		if err != nil || directory != filepath.Dir(local) {
			t.Fatalf("local directory lost: %s %v", directory, err)
		}
		from, to := "argus-openbao-0:/tmp/snapshot.bin", "private snapshot.bin"
		if upload {
			from, to = to, from
		}
		expected := append(append([]string{}, kube...), "cp", from, to, "--container", openBaoMaintenanceContainer)
		if !reflect.DeepEqual(args, expected) || !reflect.DeepEqual(kube, original) {
			t.Fatalf("file specs or shared context changed: %v", args)
		}
	}
}

func TestPodFileTransferPreservesRelativeAndMergedKubeConfig(t *testing.T) {
	paths := []string{filepath.Join("relative configs", "first"), filepath.Join("other", "second")}
	value := strings.Join(paths, string(os.PathListSeparator))
	t.Setenv("KUBECONFIG", value)
	kube := []string{"--kubeconfig", paths[0], "--context", "second"}
	args, env, err := absoluteTransferKubeConfig(kube)
	if err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		expected, err := filepath.Abs(path)
		if err != nil || filepath.SplitList(env["KUBECONFIG"])[i] != expected {
			t.Fatal("merged config path was not resolved before command Dir changed")
		}
	}
	if !filepath.IsAbs(args[1]) || kube[1] != paths[0] || os.Getenv("KUBECONFIG") != value {
		t.Fatal("caller arguments or ambient Kubernetes configuration changed")
	}
}
