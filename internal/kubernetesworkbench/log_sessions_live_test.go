package kubernetesworkbench

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type liveSessionLogBackend struct {
	liveOperationBackend
	api   kubernetes.Interface
	reads int
}

func (b *liveSessionLogBackend) ReadPinnedPodLogs(ctx context.Context, _ db.KubernetesCluster, _ uuid.UUID, q kubernetespodlogs.Query) (kubernetespodlogs.Batch, error) {
	b.reads++
	return kubernetespodlogs.Read(ctx, b.api, q)
}

// This uses a real kubelet/container runtime and real PostgreSQL, with an
// injected API adapter. It does not claim formal enrolled Connector acceptance.
func TestLivePodLogSessionsCurrentPreviousRestartAndRevocation(t *testing.T) {
	name := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if name == "" {
		t.Skip("requires explicitly selected Kubernetes context")
	}
	f, _, httpID, input := newLogFixture(t)
	grantOperationalLogs(f)
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: name}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	dynamicAPI, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	backend := &liveSessionLogBackend{api: api, liveOperationBackend: liveOperationBackend{client: kubernetesinventory.Client{Dynamic: dynamicAPI}}}
	f.service.Backend = backend
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	owner := "plan8-log-session-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	ns, err := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 45*time.Second)
		defer stop()
		current, e := api.CoreV1().Namespaces().Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.UID != ns.UID || current.Labels["argus.owner"] != owner {
			t.Errorf("namespace cleanup ownership mismatch: %v", e)
			return
		}
		if e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextCancel(cleanup, 300*time.Millisecond, true, func(ctx context.Context) (bool, error) {
			_, e := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
		t.Logf("owned namespace %s UID %s deleted", owner, ns.UID)
	})
	f.exec("UPDATE kubernetes_namespace_resources SET name=$2,namespace_uid=$3,version=version+1 WHERE id=$1", f.namespace, owner, string(ns.UID))
	no := false
	podSpec := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pinned-logs", Namespace: owner, Labels: map[string]string{"argus.owner": owner}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyAlways, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21", ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"sh", "-c", "n=$(cat /state/generation 2>/dev/null || echo 0); n=$((n+1)); echo $n > /state/generation; echo generation=$n password=plan8-live-log-canary; if [ $n -eq 1 ]; then for i in 1 2 3 4 5 6; do echo repeated-live-log; sleep 2; done; exit 0; fi; while true; do echo repeated-live-log; sleep 1; done"}, VolumeMounts: []corev1.VolumeMount{{Name: "state", MountPath: "/state"}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("8Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("32Mi")}}}}, Volumes: []corev1.Volume{{Name: "state", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}}
	pod, err := api.CoreV1().Pods(owner).Create(ctx, podSpec, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		current, e := api.CoreV1().Pods(owner).Get(ctx, pod.Name, metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		pod = current
		return len(pod.Status.ContainerStatuses) == 1 && pod.Status.ContainerStatuses[0].State.Running != nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if pod.Status.ContainerStatuses[0].RestartCount != 0 {
		t.Fatal("missed first generation; rerun the owned fixture")
	}
	input.Pod, input.PodUID, input.Previous = pod.Name, string(pod.UID), false
	first := openFixtureLog(t, f, httpID, input)
	batch, err := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, first.Session.ID, "")
	if err != nil || !strings.Contains(batch.Content, "generation=1") || strings.Contains(batch.Content, "plan8-live-log-canary") || !batch.Redacted {
		t.Fatalf("first-generation native logs: %v redacted=%v content=%q", err, batch.Redacted, batch.Content)
	}
	prior := batch.NextCursor
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 6*time.Second, true, func(ctx context.Context) (bool, error) {
		next, e := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, first.Session.ID, prior)
		if e != nil {
			return false, e
		}
		prior = next.NextCursor
		return strings.Contains(next.Content, "repeated-live-log"), nil
	}); err != nil {
		t.Fatalf("incremental logs: %v", err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		current, e := api.CoreV1().Pods(owner).Get(ctx, pod.Name, metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		pod = current
		return len(pod.Status.ContainerStatuses) == 1 && pod.Status.ContainerStatuses[0].RestartCount == 1 && pod.Status.ContainerStatuses[0].State.Running != nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	rejected, err := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, first.Session.ID, prior)
	if !errors.Is(err, kubernetesinventory.ErrIdentityChanged) || rejected.Content != "" {
		t.Fatalf("old generation continued: %v", err)
	}
	input.Previous = true
	previous := openFixtureLog(t, f, httpID, input)
	historical, err := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, previous.Session.ID, "")
	if err != nil || !historical.Complete || !strings.Contains(historical.Content, "generation=1") || strings.Contains(historical.Content, "plan8-live-log-canary") {
		t.Fatalf("previous logs: %v complete=%v", err, historical.Complete)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", f.actor.EnterpriseID, f.namespace)
	calls := backend.reads
	rejected, err = f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, previous.Session.ID, historical.NextCursor)
	if err == nil || rejected.Content != "" || backend.reads != calls {
		t.Fatal("scope revocation contacted kubelet or exposed logs")
	}
	t.Log("real current/incremental/previous logs, actual container restart, credential masking and grant-only revocation verified")
}
