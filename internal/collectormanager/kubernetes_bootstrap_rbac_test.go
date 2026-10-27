package collectormanager

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	authv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func bootstrapCollectorRBAC(t *testing.T) []*unstructured.Unstructured {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "scripts", "connector-install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "<<'EOF'")
	if start < 0 {
		t.Fatal("bootstrap manifest missing")
	}
	text = text[start:]
	start = strings.IndexByte(text, '\n')
	end := strings.Index(text, "\nEOF")
	if start < 0 || end < start {
		t.Fatal("bootstrap manifest boundary")
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(text[start+1:end]), 4096)
	result := []*unstructured.Unstructured{}
	for {
		var object unstructured.Unstructured
		err = decoder.Decode(&object)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if object.GetName() == "argus-otelcol-reader" || object.GetName() == "argus-otelcol-component-proxy" {
			result = append(result, &object)
		}
	}
	if len(result) != 4 {
		t.Fatalf("expected four collector bootstrap RBAC objects, got %d", len(result))
	}
	return result
}

func TestCollectorBootstrapMetricsRBACIsReadOnlyAndSystemScoped(t *testing.T) {
	for _, object := range bootstrapCollectorRBAC(t) {
		switch object.GetKind() {
		case "Role":
			if object.GetNamespace() != "kube-system" {
				t.Fatal("proxy access escaped system namespace")
			}
		case "ClusterRole":
			var role rbacv1.ClusterRole
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &role); err != nil {
				t.Fatal(err)
			}
			metrics := false
			for _, rule := range role.Rules {
				for _, verb := range rule.Verbs {
					if verb != "get" && verb != "list" && verb != "watch" {
						t.Fatal("observer role grants writes")
					}
				}
				for _, resource := range rule.Resources {
					if resource == "secrets" || strings.Contains(resource, "/log") || strings.Contains(resource, "/exec") || strings.Contains(resource, "/proxy") {
						t.Fatalf("observer role grants %s", resource)
					}
				}
				for _, path := range rule.NonResourceURLs {
					if path != "/metrics" {
						t.Fatal("non-resource wildcard")
					}
					metrics = true
				}
			}
			if !metrics {
				t.Fatal("native component authentication lacks metrics permission")
			}
		}
	}
}

func TestLiveCollectorBootstrapMetricsAuthorization(t *testing.T) {
	name := os.Getenv("ARGUS_KUBERNETES_COMPONENTS_CONTEXT")
	if name == "" {
		t.Skip("requires dedicated owned kind component context")
	}
	if !strings.HasPrefix(name, "kind-plan8-drain-") {
		t.Fatal("refusing bootstrap writes outside disposable kind")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: name}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	admin, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	owner := "plan8-observer-" + uuid.NewString()[:8]
	ns, err := admin.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: KubernetesNamespace, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("collector namespace must be absent before bootstrap fixture: ", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		current, e := admin.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
		if e != nil || current.UID != ns.UID || current.Labels["argus.owner"] != owner {
			t.Errorf("namespace ownership: %v", e)
			return
		}
		uid := ns.UID
		if e = admin.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := admin.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				return true, nil
			}
			return false, e
		}); e != nil {
			t.Error(e)
		}
	})
	if _, err = admin.CoreV1().ServiceAccounts(ns.Name).Create(t.Context(), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "argus-otelcol"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, object := range bootstrapCollectorRBAC(t) {
		object.SetLabels(map[string]string{"argus.owner": owner})
		switch object.GetKind() {
		case "ClusterRole":
			var value rbacv1.ClusterRole
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value)
			created, e := admin.RbacV1().ClusterRoles().Create(t.Context(), &value, metav1.CreateOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				uid := created.UID
				if e := admin.RbacV1().ClusterRoles().Delete(context.Background(), created.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			})
		case "ClusterRoleBinding":
			var value rbacv1.ClusterRoleBinding
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value)
			created, e := admin.RbacV1().ClusterRoleBindings().Create(t.Context(), &value, metav1.CreateOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				uid := created.UID
				if e := admin.RbacV1().ClusterRoleBindings().Delete(context.Background(), created.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			})
		case "Role":
			var value rbacv1.Role
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value)
			created, e := admin.RbacV1().Roles(value.Namespace).Create(t.Context(), &value, metav1.CreateOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				uid := created.UID
				if e := admin.RbacV1().Roles(created.Namespace).Delete(context.Background(), created.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			})
		case "RoleBinding":
			var value rbacv1.RoleBinding
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value)
			created, e := admin.RbacV1().RoleBindings(value.Namespace).Create(t.Context(), &value, metav1.CreateOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				uid := created.UID
				if e := admin.RbacV1().RoleBindings(created.Namespace).Delete(context.Background(), created.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			})
		}
	}
	ttl := int64(600)
	issued, err := admin.CoreV1().ServiceAccounts(ns.Name).CreateToken(t.Context(), "argus-otelcol", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &ttl}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	readerConfig := rest.AnonymousClientConfig(config)
	readerConfig.BearerToken = issued.Status.Token
	reader, err := kubernetes.NewForConfig(readerConfig)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		namespace, resource, subresource, verb string
		allow                                  bool
	}{
		{"kube-system", "pods", "proxy", "get", true}, {"default", "pods", "proxy", "get", false}, {"default", "pods", "log", "get", false}, {"default", "pods", "exec", "create", false}, {"default", "secrets", "", "get", false}, {"default", "deployments", "", "update", false}, {"", "nodes", "stats", "get", true},
	}
	for _, check := range checks {
		group := ""
		if check.resource == "deployments" {
			group = "apps"
		}
		result, e := reader.AuthorizationV1().SelfSubjectAccessReviews().Create(t.Context(), &authzv1.SelfSubjectAccessReview{Spec: authzv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authzv1.ResourceAttributes{Namespace: check.namespace, Group: group, Resource: check.resource, Subresource: check.subresource, Verb: check.verb, Name: "probe"}}}, metav1.CreateOptions{})
		if e != nil || result.Status.Allowed != check.allow {
			t.Fatalf("native %s %s/%s namespace=%s allowed=%v: %v", check.verb, check.resource, check.subresource, check.namespace, check.allow, e)
		}
	}
	result, err := reader.AuthorizationV1().SelfSubjectAccessReviews().Create(t.Context(), &authzv1.SelfSubjectAccessReview{Spec: authzv1.SelfSubjectAccessReviewSpec{NonResourceAttributes: &authzv1.NonResourceAttributes{Verb: "get", Path: "/metrics"}}}, metav1.CreateOptions{})
	if err != nil || !result.Status.Allowed {
		t.Fatal("bootstrap metrics permission denied")
	}
	t.Log("actual bootstrap manifests allow component metrics and system Pod proxy; tenant Pod proxy, business logs, exec, Secrets and writes are denied")
}
