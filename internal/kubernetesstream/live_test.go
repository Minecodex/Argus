package kubernetesstream

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Real kubelet exec/SPDY/portforward. This is a component gate, not an enrolled
// Collector, complete Plan8, or Windows acceptance claim.
func TestLiveNativeTerminalAndMultiplexedServiceForward(t *testing.T) {
	selected := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if selected == "" {
		t.Skip("requires explicitly selected Kubernetes context")
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: selected}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	owner := "plan8-stream-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
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
			t.Error("namespace cleanup ownership mismatch")
			return
		}
		if e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextCancel(cleanup, 300*time.Millisecond, true, func(c context.Context) (bool, error) {
			_, e := api.CoreV1().Namespaces().Get(c, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
		t.Logf("owned namespace %s UID %s deleted", owner, ns.UID)
	})
	no := false
	pod, err := api.CoreV1().Pods(owner).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: owner, Labels: map[string]string{"argus.owner": owner}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21", ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/bin/sh", "-ec", "exec nc -ll -p 18080 -e /bin/cat"}, Ports: []corev1.ContainerPort{{Name: "tcp", ContainerPort: 18080}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("8Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("32Mi")}}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextCancel(ctx, 300*time.Millisecond, true, func(c context.Context) (bool, error) {
		var e error
		pod, e = api.CoreV1().Pods(owner).Get(c, pod.Name, metav1.GetOptions{})
		return e == nil && len(pod.Status.ContainerStatuses) == 1 && pod.Status.ContainerStatuses[0].Ready, nil
	}); err != nil {
		t.Fatal("echo Pod readiness", err)
	}
	service, err := api.CoreV1().Services(owner).Create(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: owner}, Spec: corev1.ServiceSpec{Selector: map[string]string{"argus.owner": owner}, Ports: []corev1.ServicePort{{Name: "tcp", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt32(18080)}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := kubernetespodlogs.PinPod(string(ns.UID), pod, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	pin.PreviousContainerID = ""
	target := Target{SchemaVersion: SchemaVersion, APIServer: cfg.Host, Mode: "exec", Shell: "sh", Pin: pin, SecurityFingerprint: SecurityFingerprint(pod)}
	// Exercise the native API with a real namespaced identity, rather than the
	// administrator used only to prepare and clean up owned test resources.
	sa, err := api.CoreV1().ServiceAccounts(owner).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "stream", Namespace: owner}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	role, err := api.RbacV1().Roles(owner).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "stream", Namespace: owner}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get"}}, {APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"get", "list"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.RbacV1().RoleBindings(owner).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "stream", Namespace: owner}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: role.Name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	clusterRole, err := api.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"namespaces"}, ResourceNames: []string{owner}, Verbs: []string{"get"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		current, e := api.RbacV1().ClusterRoles().Get(cleanup, owner, metav1.GetOptions{})
		if e != nil || current.UID != clusterRole.UID || current.Labels["argus.owner"] != owner {
			t.Error("ClusterRole cleanup ownership mismatch", e)
			return
		}
		if e = api.RbacV1().ClusterRoles().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &clusterRole.UID}}); e != nil {
			t.Error(e)
		}
	})
	clusterBinding, err := api.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: owner}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		current, e := api.RbacV1().ClusterRoleBindings().Get(cleanup, owner, metav1.GetOptions{})
		if e != nil || current.UID != clusterBinding.UID || current.Labels["argus.owner"] != owner {
			t.Error("ClusterRoleBinding cleanup ownership mismatch", e)
			return
		}
		if e = api.RbacV1().ClusterRoleBindings().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &clusterBinding.UID}}); e != nil {
			t.Error(e)
		}
	})
	ttl := int64(600)
	token, err := api.CoreV1().ServiceAccounts(owner).CreateToken(ctx, sa.Name, &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &ttl}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scoped := rest.CopyConfig(cfg)
	scoped.CertFile, scoped.KeyFile, scoped.BearerTokenFile, scoped.Username, scoped.Password = "", "", "", "", ""
	scoped.CertData, scoped.KeyData = nil, nil
	scoped.ExecProvider, scoped.AuthProvider = nil, nil
	scoped.BearerToken = token.Status.Token
	if denied, e := Open(ctx, scoped, target, 100, 30); e == nil {
		_ = denied.Close(ctx, "test_cleanup")
		t.Fatal("read-only API identity opened a terminal")
	} else if !errors.Is(e, kubernetesinventory.ErrForbidden) {
		t.Fatal("read-only API denial was not distinguished from target replacement or network loss", e)
	}
	role.Rules = append(role.Rules, rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods/exec", "pods/portforward"}, Verbs: []string{"create"}})
	if _, err = api.RbacV1().Roles(owner).Update(ctx, role, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	terminal, err := Open(ctx, scoped, target, 100, 30)
	if err != nil {
		t.Fatal("native terminal open", err)
	}
	defer terminal.Close(context.Background(), "test_cleanup")
	if err = terminal.Send(ctx, interactive.Input{Type: "resize", Cols: 123, Rows: 41}); err != nil {
		t.Fatal(err)
	}
	if err = terminal.Send(ctx, interactive.Input{Type: "input", Data: "sleep 0.2; stty size; printf '__native_exec_ok__\\n'\n"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	read, c := context.WithTimeout(ctx, 10*time.Second)
	for !strings.Contains(output.String(), "41 123") || !strings.Contains(output.String(), "__native_exec_ok__\r\n") {
		f, e := terminal.Receive(read)
		if e != nil {
			c()
			t.Fatalf("native terminal output/resize missing: %v", e)
		}
		output.Write(f.Data)
	}
	c()
	_ = terminal.Close(ctx, "explicit_close")
	if err = wait.PollUntilContextCancel(ctx, 300*time.Millisecond, true, func(c context.Context) (bool, error) {
		found, e := ServiceEndpoints(c, api, owner, service, 80)
		return e == nil && found[pod.Name+"/"+string(pod.UID)] == 18080, nil
	}); err != nil {
		t.Fatal(err)
	}
	target.Mode, target.Shell, target.Port = "port_forward", "", 18080
	target.Policy.ServiceAccountUse = true
	target.Service = &ServicePin{Name: service.Name, UID: string(service.UID), Port: 80, PortName: "tcp"}
	forward, err := Open(ctx, scoped, target, 100, 30)
	if err != nil {
		t.Fatal("native service forward", err)
	}
	defer forward.Close(context.Background(), "test_cleanup")
	payloads := map[uint32][]byte{1: {0, 1, 255, 128, 'a'}, 2: []byte("second connection\n")}
	for id := uint32(1); id <= 2; id++ {
		if err = forward.Send(ctx, interactive.Input{Type: "input", Data: string(Packet(OpenChannel, id, nil))}); err != nil {
			t.Fatal(err)
		}
		if err = forward.Send(ctx, interactive.Input{Type: "input", Data: string(Packet(ChannelData, id, payloads[id]))}); err != nil {
			t.Fatal(err)
		}
		if err = forward.Send(ctx, interactive.Input{Type: "input", Data: string(Packet(ChannelEOF, id, nil))}); err != nil {
			t.Fatal(err)
		}
	}
	got := map[uint32][]byte{}
	read, c = context.WithTimeout(ctx, 10*time.Second)
	for !bytes.Equal(got[1], payloads[1]) || !bytes.Equal(got[2], payloads[2]) {
		f, e := forward.Receive(read)
		if e != nil {
			c()
			t.Fatal("multiplex byte/half-close response", e)
		}
		if f.Type != "output" {
			c()
			t.Fatal("forward ended early", f.Status, f.Reason)
		}
		k, id, data, e := ParsePacket(f.Data)
		if e != nil {
			c()
			t.Fatal(e)
		}
		if k == ChannelData {
			got[id] = append(got[id], data...)
		}
	}
	c()
	if err = api.CoreV1().Services(owner).Delete(ctx, service.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &service.UID}}); err != nil {
		t.Fatal(err)
	}
	read, c = context.WithTimeout(ctx, 10*time.Second)
	defer c()
	for {
		f, e := forward.Receive(read)
		if e != nil {
			t.Fatal("service replacement did not invalidate stream", e)
		}
		if f.Type == "state" {
			if f.Status != "invalidated" {
				t.Fatal(f)
			}
			break
		}
	}
	t.Log("real namespaced API identity: read-only terminal denial, TTY output, 123x41 resize, two byte-exact TCP channels, half-close and Service UID invalidation passed")
}
