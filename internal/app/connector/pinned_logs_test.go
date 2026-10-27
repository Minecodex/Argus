package connector

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/connectorprotocol"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestPinnedLogCommandProtocolAndPrivateResultProjection(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer controlled-native-token" {
			t.Error("missing native Kubernetes authentication")
			http.Error(w, "denied", 403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/orders":
			_ = json.NewEncoder(w).Encode(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orders", UID: "ns-uid"}})
		case "/api/v1/namespaces/orders/pods/api":
			_ = json.NewEncoder(w).Encode(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: "pod-uid"}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://main", RestartCount: 0}}}})
		case "/api/v1/namespaces/orders/pods/api/log":
			_, _ = fmt.Fprint(w, "2026-10-19T10:00:01Z token=protocol-log-canary\n")
		default:
			t.Errorf("unexpected target %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	config := clientcmdapi.Config{CurrentContext: "fixture", Clusters: map[string]*clientcmdapi.Cluster{"fixture": {Server: "https://unselected.example.test", CertificateAuthorityData: ca}}, AuthInfos: map[string]*clientcmdapi.AuthInfo{"fixture": {Token: "controlled-native-token"}}, Contexts: map[string]*clientcmdapi.Context{"fixture": {Cluster: "fixture", AuthInfo: "fixture"}}}
	credential, err := clientcmd.Write(config)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(credential)
	query := kubernetespodlogs.Query{APIServer: server.URL, ContinuityKey: bytes.Repeat([]byte{3}, 32), Pin: kubernetespodlogs.Pin{Namespace: "orders", NamespaceUID: "ns-uid", Pod: "api", PodUID: "pod-uid", Container: "main", ContainerID: "containerd://main"}, Since: time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC), TailLines: 10}
	raw, _ := json.Marshal(query)
	request := &connectorv1.KubernetesPinnedPodLogs{SchemaVersion: kubernetespodlogs.SchemaVersion, ClusterId: uuid.NewString(), SessionId: uuid.NewString(), QueryJson: raw}
	call := func() *connectorv1.KubernetesPinnedPodLogsResult {
		payload, e := anypb.New(request)
		if e != nil {
			t.Fatal(e)
		}
		result, e := executePinnedPodLogs(t.Context(), payload, credential)
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	request.SchemaVersion = "argus.kubernetes_pod_logs/v0"
	if result := call(); result.ErrorCode != kubernetesinventory.ErrProtocol.Error() || calls != 0 {
		t.Fatal("legacy protocol accessed Kubernetes")
	}
	request.SchemaVersion = kubernetespodlogs.SchemaVersion
	result := call()
	if result.ErrorCode != "" || !bytes.Contains(result.BatchJson, []byte("REDACTED")) || bytes.Contains(result.BatchJson, []byte("protocol-log-canary")) || bytes.Contains(result.BatchJson, []byte("controlled-native-token")) {
		t.Fatal("native typed logs failed or private data reached the command result")
	}
	value, e := anypb.New(result)
	if e != nil {
		t.Fatal(e)
	}
	if !connectorprotocol.ResultAllowed("kubernetes_pinned_pod_logs", value) {
		t.Fatal("result factory did not accept new command")
	}
	query.Pin.PodUID = "replacement-uid"
	request.QueryJson, _ = json.Marshal(query)
	result = call()
	if result.ErrorCode != kubernetesinventory.ErrIdentityChanged.Error() || len(result.BatchJson) != 0 {
		t.Fatal("substituted UID exposed log data")
	}
	request.QueryJson = []byte(strings.Repeat("x", 17<<10))
	if result = call(); result.ErrorCode != kubernetesinventory.ErrInvalid.Error() {
		t.Fatal("unbounded query accepted")
	}
}
