package kubernetesruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
)

type liveKubeFixture struct {
	api              kubernetes.Interface
	config           *rest.Config
	credential       []byte
	namespace, image string
}

func newLiveKubeFixture(t *testing.T) *liveKubeFixture {
	t.Helper()
	if os.Getenv("ARGUS_P7_KUBE_LIVE") != "1" {
		t.Skip("requires owned live Kubernetes fixture opt-in")
	}
	image := os.Getenv("ARGUS_P7_REMOTE_IMAGE")
	if image == "" {
		t.Fatal("ARGUS_P7_REMOTE_IMAGE is required")
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	raw, err := loader.RawConfig()
	if err != nil {
		t.Fatal(err)
	}
	expected := os.Getenv("ARGUS_P7_KUBE_CONTEXT")
	if expected == "" {
		expected = "docker-desktop"
	}
	if raw.CurrentContext != expected {
		t.Fatalf("test context mismatch: %s", raw.CurrentContext)
	}
	config, err := loader.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err = rest.LoadTLSFiles(config); err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	name := "argus-p7-" + uuid.NewString()[:8]
	ns, err := api.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/plan": "plan7", "argus.io/owner": "codex-plan7", "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "latest"}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("owned namespace=%s uid=%s", name, ns.UID)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		current, e := api.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			t.Error(e)
			return
		}
		if current.UID != ns.UID || current.Labels["argus.io/owner"] != "codex-plan7" {
			t.Error("namespace ownership changed; retained")
			return
		}
		if e = api.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); e != nil {
			t.Error(e)
		}
	})
	no := false
	if _, err = api.CoreV1().ServiceAccounts(name).Create(t.Context(), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "operator"}, AutomountServiceAccountToken: &no}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "operator"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods", "pods/exec", "pods/log", "configmaps"}, Verbs: []string{"get", "list", "create", "update", "patch", "delete"}}, {APIGroups: []string{"networking.k8s.io"}, Resources: []string{"networkpolicies"}, Verbs: []string{"get", "list", "create", "update", "delete"}}}}
	if _, err = api.RbacV1().Roles(name).Create(t.Context(), role, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = api.RbacV1().RoleBindings(name).Create(t.Context(), &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "operator"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "operator", Namespace: name}}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "operator"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	expires := int64(600)
	token, err := api.CoreV1().ServiceAccounts(name).CreateToken(t.Context(), "operator", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &expires}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := clientcmd.Write(clientapi.Config{Clusters: map[string]*clientapi.Cluster{"fixture": {Server: config.Host, CertificateAuthorityData: config.CAData}}, AuthInfos: map[string]*clientapi.AuthInfo{"operator": {Token: token.Status.Token}}, Contexts: map[string]*clientapi.Context{"fixture": {Cluster: "fixture", AuthInfo: "operator", Namespace: name}}, CurrentContext: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGUS_REMOTE_NAMESPACE", name)
	t.Setenv("ARGUS_REMOTE_KUBECTL_IMAGE", image)
	t.Cleanup(func() { clear(credential) })
	return &liveKubeFixture{api: api, config: config, credential: credential, namespace: name, image: image}
}
func liveKubeBinding(f *liveKubeFixture, interpreter string) *connectorv1.RemoteExecutionBatch {
	return &connectorv1.RemoteExecutionBatch{ResourceType: "kubernetes_cluster", ResourceId: uuid.NewString(), KubernetesApiServer: f.config.Host, Interpreter: interpreter, RequestId: uuid.NewString(), SessionId: uuid.NewString(), SessionGeneration: uuid.NewString(), IdentityBinding: uuid.NewString(), Username: "fixture-operator", ApprovedContentHash: strings.Repeat("a", 64), ActionRef: "act_fixture_approved", TimeoutSeconds: 30, MaxOutputBytes: 64 << 10, Sequence: 1}
}
func liveKubeBatch(t *testing.T, bridge *kubernetesRuntimeBridge, b *connectorv1.RemoteExecutionBatch, script string) *connectorv1.RemoteExecutionBatchResult {
	t.Helper()
	b.Script = script
	hash := sha256.Sum256([]byte(script))
	b.ScriptSha256 = hex.EncodeToString(hash[:])
	result, err := bridge.call(t.Context(), "remote_execution_batch", b)
	if err != nil {
		t.Fatal(err)
	}
	receipt := result.(*connectorv1.RemoteExecutionBatchResult)
	if receipt.ExitCode != 0 || receipt.SessionLost {
		t.Fatalf("batch failed: exit=%d code=%s stderr=%s", receipt.ExitCode, receipt.ErrorCode, receipt.Stderr)
	}
	return receipt
}
func openLiveKube(t *testing.T, f *liveKubeFixture, b *connectorv1.RemoteExecutionBatch) *kubernetesRuntimeBridge {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	bridge, err := openKubernetesRuntime(ctx, b, f.credential)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bridge.wire.Close)
	opened, err := bridge.call(ctx, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "open", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Binding: b})
	if err != nil {
		t.Fatal(err)
	}
	if opened.(*connectorv1.RemoteExecutionControlResult).RuntimeAccount == "" {
		t.Fatal("container identity absent")
	}
	return bridge
}
func TestLivePodRuntimeStateFilesAndReplacement(t *testing.T) {
	f := newLiveKubeFixture(t)
	no := false
	yes := true
	uid := int64(65532)
	pod, err := f.api.CoreV1().Pods(f.namespace).Create(t.Context(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "target"}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyNever, SecurityContext: &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Containers: []corev1.Container{{Name: "target", Image: f.image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/bin/bash", "-c", "exec sleep infinity"}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	probe := &kubernetesRuntimeBridge{api: f.api}
	if _, err = probe.waitRunningPod(t.Context(), pod, 90*time.Second); err != nil {
		t.Fatal(err)
	}
	b := liveKubeBinding(f, "pod_bash")
	b.Namespace = f.namespace
	b.Pod = pod.Name
	b.PodUid = string(pod.UID)
	b.Container = "target"
	bridge := openLiveKube(t, f, b)
	liveKubeBatch(t, bridge, b, "value=41; cd \"$ARGUS_OUTPUT_DIR\"; printf 'first' > first.txt")
	b.ContextRevision = 1
	b.Sequence = 2
	receipt := liveKubeBatch(t, bridge, b, "value=$((value+1)); printf '%s' \"$value\" > \"$ARGUS_OUTPUT_DIR/answer.txt\"; printf '%s' \"$value\"")
	if string(receipt.Stdout) != "42" {
		t.Fatal("Pod shell state was lost")
	}
	listing, err := bridge.call(t.Context(), "remote_execution_file", &connectorv1.RemoteExecutionFile{Operation: "list_output", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range listing.(*connectorv1.RemoteExecutionFileResult).Files {
		if file.Path == "answer.txt" && file.ByteSize == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("automatic output missing")
	}
	recovered := recoverLiveFiles(t, f, b, bridge, "answer.txt")
	if receipt, err := recovered.call(t.Context(), "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "reclaim", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Binding: b}); err != nil || receipt.(*connectorv1.RemoteExecutionControlResult).Status != "reclaimed" {
		t.Fatalf("Pod helper reclaim failed: %v", err)
	}
	verifyContext, stopVerification := context.WithTimeout(t.Context(), 10*time.Second)
	defer stopVerification()
	if err := bridge.exec(verifyContext, []string{"sh", "-c", "test ! -e /tmp/argus-runtime/" + b.SessionId + "/" + b.SessionGeneration + " && test ! -e /tmp/argus/" + b.SessionId + "/" + b.SessionGeneration}, nil, io.Discard, io.Discard); err != nil {
		t.Fatalf("owned helper or exchange was not reclaimed: %v", err)
	}
	zero := int64(0)
	if err = f.api.CoreV1().Pods(f.namespace).Delete(t.Context(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &pod.UID}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err = f.api.CoreV1().Pods(f.namespace).Get(t.Context(), pod.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("owned Pod did not terminate: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	replacement, err := f.api.CoreV1().Pods(f.namespace).Create(t.Context(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: pod.Name}, Spec: pod.Spec}, metav1.CreateOptions{})
	if err != nil || replacement.UID == pod.UID {
		t.Fatalf("Pod replacement failed: %v", err)
	}
	if _, err = probe.waitRunningPod(t.Context(), replacement, 90*time.Second); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual Pod replacement old_uid=%s new_uid=%s", pod.UID, replacement.UID)
	if _, err = recovered.call(t.Context(), "remote_execution_file", &connectorv1.RemoteExecutionFile{Operation: "list_output", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId}); err == nil || err.Error() != "REMOTE_POD_CHANGED" {
		t.Fatalf("replacement did not invalidate the pinned Pod UID: %v", err)
	}
}
func TestLiveKubectlRuntimeIdentityAndNetworkIsolation(t *testing.T) {
	f := newLiveKubeFixture(t)
	b := liveKubeBinding(f, "kubectl")
	bridge := openLiveKube(t, f, b)
	script := "kubectl create configmap plan7-result --from-literal=value=approved -n " + f.namespace + "; kubectl get configmap plan7-result -n " + f.namespace + " -o json > \"$ARGUS_OUTPUT_DIR/result.json\""
	liveKubeBatch(t, bridge, b, script)
	item, err := f.api.CoreV1().ConfigMaps(f.namespace).Get(t.Context(), "plan7-result", metav1.GetOptions{})
	if err != nil || item.Data["value"] != "approved" {
		t.Fatal("kubectl did not use approved identity", err)
	}
	b.ContextRevision = 1
	b.Sequence = 2
	receipt := liveKubeBatch(t, bridge, b, "test ! -e /var/run/secrets/kubernetes.io/serviceaccount/token; ! grep -Eq 'token:|client-key|client-certificate' \"$KUBECONFIG\"; ! kubectl get pods -n kube-system >/dev/null 2>&1; ! kubectl --as=admin get pods >/dev/null 2>&1")
	if receipt.ExitCode != 0 {
		t.Fatal("credential isolation or RBAC failed")
	}
	recovered := recoverLiveFiles(t, f, b, bridge, "result.json")
	result, err := recovered.call(t.Context(), "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "reclaim", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Binding: b})
	if err != nil || result.(*connectorv1.RemoteExecutionControlResult).Status != "reclaimed" {
		t.Fatalf("reclaim: %v", err)
	}
	// NetworkPolicy is additive: an existing allow-all must make the runtime
	// unavailable, never turn a nominal deny policy into a false safety claim.
	if _, err = f.api.NetworkingV1().NetworkPolicies(f.namespace).Create(t.Context(), &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "conflicting-allow"}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{}}, Egress: []networkingv1.NetworkPolicyEgressRule{{}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if other, err := openKubernetesRuntime(t.Context(), liveKubeBinding(f, "kubectl"), f.credential); err == nil {
		other.Close()
		t.Fatal("conflicting allow policy bypassed isolation")
	} else if err.Error() != "REMOTE_NETWORK_ISOLATION_UNAVAILABLE" {
		t.Fatal("wrong isolation failure", err)
	}
}

func recoverLiveFiles(t *testing.T, f *liveKubeFixture, b *connectorv1.RemoteExecutionBatch, original *kubernetesRuntimeBridge, expected string) *kubernetesRuntimeBridge {
	t.Helper()
	original.Close()
	recovered, err := Recover(t.Context(), b, f.credential)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(recovered.Close)
	control := &connectorv1.RemoteExecutionControl{Operation: "recover_files", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId, Binding: b}
	receipt, err := recovered.call(t.Context(), "remote_execution_control", control)
	if err != nil || receipt.(*connectorv1.RemoteExecutionControlResult).Status != "files_only" {
		t.Fatalf("file recovery: %v", err)
	}
	listed, err := recovered.call(t.Context(), "remote_execution_file", &connectorv1.RemoteExecutionFile{Operation: "list_output", SessionId: b.SessionId, SessionGeneration: b.SessionGeneration, RequestId: b.RequestId})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range listed.(*connectorv1.RemoteExecutionFileResult).Files {
		if item.Path == expected {
			found = true
		}
	}
	if !found {
		t.Fatalf("recovery lost %s", expected)
	}
	if _, err = recovered.call(t.Context(), "remote_execution_batch", b); err == nil {
		t.Fatal("file recovery revived executable context")
	}
	control.Operation = "open"
	if _, err = recovered.call(t.Context(), "remote_execution_control", control); err == nil {
		t.Fatal("file recovery accepted interpreter creation")
	}
	return recovered
}
