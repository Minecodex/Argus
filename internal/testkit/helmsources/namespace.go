package helmsources

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func Namespace(t *testing.T) (context.Context, *rest.Config, kubernetes.Interface, *corev1.Namespace) {
	t.Helper()
	selected := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if selected == "" {
		t.Skip("requires selected isolated Kubernetes context")
	}
	config, e := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: selected}).ClientConfig()
	if e != nil {
		t.Fatal(e)
	}
	config.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	api, e := kubernetes.NewForConfig(config)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	t.Cleanup(cancel)
	owner := "plan8-helm-source-" + uuid.NewString()[:10]
	ns, e := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		current, e := api.CoreV1().Namespaces().Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.UID != ns.UID || current.Labels["argus.owner"] != owner {
			t.Errorf("namespace ownership: %v", e)
			return
		}
		if e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 250*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
	})
	return ctx, config, api, ns
}
