package kubernetesinventory

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Real Kubernetes API/projection gate in an owned disposable namespace. It is
// not the Argus control-plane authorization or Connector end-to-end gate.
func TestLiveInventoryPaginationProjectionAndNamespaceRecreation(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("requires explicit isolated Kubernetes context")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	owner := "plan8-inventory-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	createNamespace := func() *corev1.Namespace {
		t.Helper()
		value, e := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	namespace := createNamespace()
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		value, e := api.CoreV1().Namespaces().Get(cleanup, owner, metav1.GetOptions{})
		if e == nil && value.Labels["argus.owner"] == owner {
			if e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &value.UID}}); e != nil {
				t.Errorf("owned namespace cleanup: %v", e)
			}
			if e = wait.PollUntilContextTimeout(cleanup, 250*time.Millisecond, 25*time.Second, true, func(ctx context.Context) (bool, error) {
				_, err := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}); e != nil {
				t.Errorf("owned namespace deletion not confirmed: %v", e)
			}
		}
	})
	for _, name := range []string{"first", "second", "third"} {
		_, err = api.CoreV1().ConfigMaps(owner).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.owner": owner}}, Data: map[string]string{"value": "observed"}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := Client{Dynamic: dynamicClient}
	query := Query{Type: "configmap", Namespace: owner, NamespaceUID: string(namespace.UID), Labels: "argus.owner=" + owner, Limit: 1}
	seen := map[string]bool{}
	pages := 0
	resourceVersion := ""
	for {
		page, e := reader.List(ctx, query)
		if e != nil {
			t.Fatal(e)
		}
		pages++
		resourceVersion = page.ResourceVersion
		if len(page.Items) != 1 || page.ResourceVersion == "" {
			t.Fatal("native pagination missing")
		}
		object := page.Items[0]
		if seen[object.UID] || object.NamespaceUID != string(namespace.UID) {
			t.Fatal("page identity duplicated or lost")
		}
		seen[object.UID] = true
		if page.Complete {
			break
		}
		query.Continue = page.Continue
		if pages > 5 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != 3 || pages != 3 {
		t.Fatalf("pagination lost objects: %d %d", len(seen), pages)
	}
	watchResults := make(chan WatchBatch, 1)
	watchErrors := make(chan error, 1)
	go func() {
		batch, err := reader.Watch(ctx, Query{Type: "configmap", Namespace: owner, NamespaceUID: string(namespace.UID), Labels: "argus.owner=" + owner, ResourceVersion: resourceVersion, Limit: 1})
		if err != nil {
			watchErrors <- err
		} else {
			watchResults <- batch
		}
	}()
	_, err = api.CoreV1().ConfigMaps(owner).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "watch-added", Labels: map[string]string{"argus.owner": owner}}, Data: map[string]string{"value": "fresh"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case batch := <-watchResults:
		if len(batch.Events) != 1 || batch.Events[0].Object.Name != "watch-added" || batch.Events[0].Type != "ADDED" || batch.ResourceVersion == resourceVersion {
			t.Fatalf("list/watch gap: %+v", batch)
		}
	case err := <-watchErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	secretValue := "plan8-private-test-value"
	secret, err := api.CoreV1().Secrets(owner).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "private", Labels: map[string]string{"argus.owner": owner}}, Data: map[string][]byte{"opaque": []byte(secretValue)}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	object, err := reader.Get(ctx, Query{Type: "secret", Namespace: owner, NamespaceUID: string(namespace.UID), Name: secret.Name, UID: string(secret.UID)})
	if err != nil {
		t.Fatal(err)
	}
	if !object.Redacted || strings.Contains(string(object.Data), secretValue) || strings.Contains(string(object.Data), base64.StdEncoding.EncodeToString([]byte(secretValue))) {
		t.Fatal("live Secret crossed projection")
	}
	oldUID := namespace.UID
	if err = api.CoreV1().Namespaces().Delete(ctx, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &oldUID}}); err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, e := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
		return apierrors.IsNotFound(e), nil
	}); err != nil {
		t.Fatal(err)
	}
	namespace = createNamespace()
	if namespace.UID == oldUID {
		t.Fatal("namespace identity did not change")
	}
	_, err = reader.List(ctx, Query{Type: "pod", Namespace: owner, NamespaceUID: string(oldUID), Limit: 10})
	if !errors.Is(err, ErrIdentityChanged) {
		t.Fatalf("old namespace authorization inherited: %v", err)
	}
	t.Log("three server pages preserved all object identities; Secret projected before transport; recreated namespace rejected old UID")
}
