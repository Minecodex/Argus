package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type liveBehaviorFixture struct {
	f      *mutationFixture
	client *kubernetes.Clientset
	config *rest.Config
	name   string
}

func newLiveBehaviorFixture(t *testing.T, env string) liveBehaviorFixture {
	t.Helper()
	contextName := os.Getenv(env)
	if contextName == "" {
		t.Skip("requires explicit runtime behavior context: " + env)
	}
	f := newMutationFixture(t)
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	native, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	name := "plan8-behavior-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	ns, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.owner": name}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		current, e := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.UID != ns.UID || current.Labels["argus.owner"] != name {
			t.Error("behavior Namespace ownership changed", e)
			return
		}
		uid := ns.UID
		if e = client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 80*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error("behavior cleanup", e)
		}
	})
	f.exec("UPDATE kubernetes_namespace_resources SET name=$1,namespace_uid=$2 WHERE id=$3", name, string(ns.UID), f.namespace)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT b.role_id,p.id FROM role_bindings b CROSS JOIN permissions p WHERE b.enterprise_id=$1 AND p.id IN ('kubernetes.workload.manage','kubernetes.network.manage') ON CONFLICT DO NOTHING", f.actor.EnterpriseID)
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: native}, secrets: f.service.Credentials}
	return liveBehaviorFixture{f: f, client: client, config: config, name: name}
}

func (b liveBehaviorFixture) apply(t *testing.T, kind, name, operation string, body map[string]any) {
	t.Helper()
	input := MutationInput{ClusterID: b.f.cluster, NamespaceID: b.f.namespace, Type: kind, Name: name, Operation: operation}
	if body != nil {
		var err error
		input.Object, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	if operation != "create" {
		d, _ := kubernetesinventory.Lookup(kind)
		backend := b.f.service.Backend.(liveOperationBackend)
		current, e := backend.client.Dynamic.Resource(d.GVR()).Namespace(b.name).Get(t.Context(), name, metav1.GetOptions{})
		if e != nil {
			t.Fatal(e)
		}
		input.UID, input.ExpectedVersion = string(current.GetUID()), current.GetResourceVersion()
	}
	id, execution := b.f.queueInput(input)
	w := Worker{Service: b.f.service}
	if err := w.execute(t.Context(), operationTask{b.f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(t.Context(), 250*time.Millisecond, 70*time.Second, true, func(ctx context.Context) (bool, error) {
		row := b.f.operation(id)
		if row.Status == "succeeded" {
			return true, nil
		}
		if row.Status == "failed" || row.Status == "result_unknown" {
			return false, fmt.Errorf("%s %s failed: %s", kind, operation, row.ErrorCode)
		}
		return false, w.observe(ctx, row)
	}); err != nil {
		t.Fatal(err)
	}
	b.f.reconcileExecution(execution)
}

func (b liveBehaviorFixture) exec(t *testing.T, pod string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	request := b.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(b.name).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "main", Command: args, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(b.config, http.MethodPost, request.URL())
	if err != nil {
		return "", err
	}
	var out, diagnostic bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &out, Stderr: &diagnostic})
	return strings.TrimSpace(out.String()), err
}

func TestLiveNetworkPolicyApprovalControlsActualTraffic(t *testing.T) {
	b := newLiveBehaviorFixture(t, "ARGUS_KUBERNETES_POLICY_CONTEXT")
	for _, role := range []string{"server", "client"} {
		command := []any{"sleep", "600"}
		if role == "server" {
			command = []any{"sh", "-c", "mkdir -p /www; printf policy-proof > /www/index.html; exec busybox httpd -f -p 8080 -h /www"}
		}
		b.apply(t, "pod", role, "create", map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": role, "namespace": b.name, "labels": map[string]any{"role": role}}, "spec": map[string]any{"automountServiceAccountToken": false, "terminationGracePeriodSeconds": int64(0), "containers": []any{map[string]any{"name": "main", "image": "busybox:1.36", "imagePullPolicy": "Never", "command": command}}}})
	}
	server, err := b.client.CoreV1().Pods(b.name).Get(t.Context(), "server", metav1.GetOptions{})
	if err != nil || server.Status.PodIP == "" {
		t.Fatal("server has no actual Pod IP", err)
	}
	probe := func(want string) error {
		return wait.PollUntilContextTimeout(t.Context(), 300*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			actual, e := b.exec(t, "client", "sh", "-c", "if wget -qO- -T 2 http://"+server.Status.PodIP+":8080; then printf '|allowed'; else printf 'denied'; fi")
			if e != nil {
				return false, e
			}
			return actual == want, nil
		})
	}
	if err := probe("policy-proof|allowed"); err != nil {
		t.Fatal("baseline connectivity failed", err)
	}
	b.apply(t, "networkpolicy", "isolation", "create", map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]any{"name": "isolation", "namespace": b.name}, "spec": map[string]any{"podSelector": map[string]any{"matchLabels": map[string]any{"role": "server"}}, "policyTypes": []any{"Ingress"}, "ingress": []any{}}})
	if err := probe("denied"); err != nil {
		t.Fatal("API accepted policy without denying actual traffic", err)
	}
	b.apply(t, "networkpolicy", "isolation", "patch", map[string]any{"spec": map[string]any{"ingress": []any{map[string]any{"from": []any{map[string]any{"podSelector": map[string]any{"matchLabels": map[string]any{"role": "client"}}}}}}}})
	if err := probe("policy-proof|allowed"); err != nil {
		t.Fatal("allow policy did not restore connectivity", err)
	}
	b.apply(t, "networkpolicy", "isolation", "delete", nil)
	if err := probe("policy-proof|allowed"); err != nil {
		t.Fatal("policy deletion changed baseline connectivity", err)
	}
	t.Log("actual traffic allowed, denied, selectively restored and preserved after deletion through PostgreSQL preview/confirm/worker; native transport remains a controlled adapter")
}
