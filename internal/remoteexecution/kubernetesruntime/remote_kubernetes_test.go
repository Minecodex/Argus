package kubernetesruntime

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kakj-go/Argus/internal/remoteexecution/runtimewire"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestKubernetesProxyPinsServerAndKeepsCredentialsInConnector(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer connector-only-secret" {
			t.Error("credential was not applied by trusted transport")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://other.invalid/", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","items":[]}`))
	}))
	defer server.Close()
	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "runtime", Namespace: "test", UID: types.UID("pinned")}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "commands", ContainerID: "containerd://fixed", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}}
	bridge := &kubernetesRuntimeBridge{api: fake.NewClientset(pod), config: &rest.Config{Host: server.URL, BearerToken: "connector-only-secret", TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})}}, namespace: "test", pod: "runtime", podUID: "pinned", container: "commands", containerID: "containerd://fixed"}
	response := bridge.proxy(t.Context(), runtimewire.ProxyRequest{Method: "GET", URI: "/api/v1/pods", Header: http.Header{}})
	if response.Status != 200 || strings.Contains(string(response.Body), "connector-only-secret") || calls.Load() != 1 {
		t.Fatalf("proxy response: %#v", response)
	}
	for _, request := range []runtimewire.ProxyRequest{{Method: "GET", URI: "https://other.invalid/api"}, {Method: "GET", URI: "//other.invalid/api"}, {Method: "GET", URI: "/api", Header: http.Header{"Impersonate-User": []string{"admin"}}}, {Method: "GET", URI: "/api", Header: http.Header{"Authorization": []string{"Bearer replacement"}}}} {
		if got := bridge.proxy(t.Context(), request); got.Status != 403 {
			t.Fatalf("connection override accepted: %#v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("forbidden request reached an API server")
	}
	if got := bridge.proxy(t.Context(), runtimewire.ProxyRequest{Method: "GET", URI: "/redirect"}); got.Status != 307 {
		t.Fatal("followed API redirect")
	}
	pod.UID = "replacement"
	if _, err = bridge.api.CoreV1().Pods("test").Update(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = bridge.validate(t.Context()); err == nil {
		t.Fatal("same name with replacement UID accepted")
	}
}
