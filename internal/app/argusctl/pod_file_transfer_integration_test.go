package argusctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLivePodFileTransferPreservesBinaryAndArchiveAcrossLocalDrives(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_KUBECTL_TRANSFER") != "1" {
		t.Skip("explicit owned Kubernetes file transfer integration required")
	}
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Fatal("explicit Kubernetes context required")
	}
	clients, err := clientsFor(contextName)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// A relative kubeconfig must continue to resolve from the caller's CWD,
	// even while kubectl copies a file beside the user's temp directory.
	if config := os.Getenv("KUBECONFIG"); config != "" {
		paths := filepath.SplitList(config)
		for i, path := range paths {
			if path == "" {
				continue
			}
			if relative, e := filepath.Rel(cwd, path); e == nil {
				paths[i] = relative
			}
		}
		t.Setenv("KUBECONFIG", strings.Join(paths, string(os.PathListSeparator)))
	}
	ambient := os.Getenv("KUBECONFIG")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	owner := "argus-file-transfer-" + uuid.NewString()[:8]
	namespace, err := clients.typed.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.io/file-transfer-owner": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		current, e := clients.typed.CoreV1().Namespaces().Get(clean, owner, metav1.GetOptions{})
		if e != nil || current.UID != namespace.UID || current.Labels["argus.io/file-transfer-owner"] != owner {
			t.Error("refuse to remove a changed file transfer namespace")
			return
		}
		if e = clients.typed.CoreV1().Namespaces().Delete(clean, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &namespace.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextCancel(clean, 250*time.Millisecond, true, func(check context.Context) (bool, error) {
			_, e := clients.typed.CoreV1().Namespaces().Get(check, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error("owned transfer namespace cleanup", e)
		}
	})
	uid, no, yes := int64(65532), false, true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "argus-openbao-0", Namespace: owner}, Spec: corev1.PodSpec{
		AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyNever,
		SecurityContext: &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid, RunAsNonRoot: &yes},
		Volumes:         []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}, {Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
	}}
	for _, entry := range []struct{ name, mount string }{{"server", "/primary"}, {openBaoMaintenanceContainer, "/secondary"}} {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
			Name: entry.name, Image: "busybox:1.36", ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sh", "-ec", "sleep 3600"},
			SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &yes, AllowPrivilegeEscalation: &no},
			VolumeMounts:    []corev1.VolumeMount{{Name: "data", MountPath: entry.mount}, {Name: "tmp", MountPath: "/tmp"}},
		})
	}
	if _, err = clients.typed.CoreV1().Pods(owner).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(check context.Context) (bool, error) {
		current, e := clients.typed.CoreV1().Pods(owner).Get(check, pod.Name, metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		ready := len(current.Status.ContainerStatuses) == 2
		for _, status := range current.Status.ContainerStatuses {
			ready = ready && status.Ready
		}
		return ready, nil
	}); err != nil {
		t.Fatal("owned transfer fixture did not become ready", err)
	}
	directory := filepath.Join(t.TempDir(), "snapshot files with spaces")
	if err = os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, 256)
	for i := range seed {
		seed[i] = byte(i)
	}
	payload := bytes.Repeat(seed, 8192)
	source := filepath.Join(directory, "private source.bin")
	if err = os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	a := App{stdout: io.Discard, stderr: io.Discard, runner: commandRunner{stdout: io.Discard, stderr: io.Discard}}
	kube := []string{"--context", contextName, "--namespace", owner}
	file := podFileTransfer{Pod: pod.Name, Container: openBaoMaintenanceContainer, RemotePath: "/secondary/payload.bin", LocalPath: source, ToPod: true}
	if err = a.transferPodFile(ctx, kube, file); err != nil {
		t.Fatal("actual secondary-container upload", err)
	}
	file.LocalPath, file.ToPod = filepath.Join(directory, "restored snapshot.bin"), false
	if err = a.transferPodFile(ctx, kube, file); err != nil {
		t.Fatal("actual secondary-container download", err)
	}
	actual, err := os.ReadFile(file.LocalPath)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("binary snapshot bytes changed")
	}
	archive := filepath.Join(directory, "private data archive.tgz")
	if err = a.capturePodArchive(ctx, kube, pod.Name, "/primary", archive); err != nil {
		t.Fatal("actual backup archive download", err)
	}
	if _, err = a.runner.quiet(ctx, "kubectl", append(kube, "exec", pod.Name, "--container", "server", "--", "rm", "/primary/payload.bin")...); err != nil {
		t.Fatal(err)
	}
	if err = a.restorePodArchive(ctx, kube, pod.Name, archive, "/primary"); err != nil {
		t.Fatal("actual archive restore", err)
	}
	remote, err := a.runner.quiet(ctx, "kubectl", append(kube, "exec", pod.Name, "--container", "server", "--", "sha256sum", "/primary/payload.bin")...)
	if err != nil || !strings.HasPrefix(remote, fmt.Sprintf("%x", sha256.Sum256(payload))) {
		t.Fatal("restored archive changed the original binary payload")
	}
	current, err := os.Getwd()
	if err != nil || current != cwd || os.Getenv("KUBECONFIG") != ambient {
		t.Fatal("file transfer mutated caller working directory or Kubernetes configuration")
	}
	t.Logf("actual binary upload/download and archive restore matched 2 MiB; local drive=%s caller drive=%s; secondary container, relative config and unchanged CWD verified", filepath.VolumeName(directory), filepath.VolumeName(cwd))
}
