package kubernetesinventory

import (
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	kubetest "k8s.io/client-go/testing"
)

func TestWatchBoundsRedactionAndExpiredVersion(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "projected_batch", true: "expired_requires_relist"}[expired], func(t *testing.T) {
			namespace := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "owned", "uid": "ns-uid", "resourceVersion": "1"}}}
			client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespace)
			events := watch.NewRaceFreeFake()
			started := make(chan struct{})
			client.PrependWatchReactor("secrets", func(kubetest.Action) (bool, watch.Interface, error) { close(started); return true, events, nil })
			go func() {
				<-started
				if expired {
					events.Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
					return
				}
				events.Add(&unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "private", "namespace": "owned", "uid": "object-uid", "resourceVersion": "2"}, "data": map[string]any{"opaque": "private-watch-canary"}}})
			}()
			result, err := (Client{Dynamic: client}).Watch(t.Context(), Query{Type: "secret", Namespace: "owned", NamespaceUID: "ns-uid", ResourceVersion: "1", Limit: 1})
			if expired {
				if !errors.Is(err, ErrExpired) || len(result.Events) != 0 {
					t.Fatalf("expired watch changed scope: %+v %v", result, err)
				}
				return
			}
			if err != nil || len(result.Events) != 1 || result.ResourceVersion != "2" || !result.More {
				t.Fatalf("bounded watch: %+v %v", result, err)
			}
			if strings.Contains(string(result.Events[0].Object.Data), "private-watch-canary") || !result.Events[0].Object.Redacted {
				t.Fatal("watch leaked Secret data")
			}
		})
	}
}
