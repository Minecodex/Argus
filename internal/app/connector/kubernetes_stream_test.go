package connector

import (
	"encoding/json"
	"testing"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
)

func TestKubernetesInteractiveProtocolRejectsUnboundAndCrossProtocolTargets(t *testing.T) {
	if len(interactiveCapabilities("host", []string{"host.local_shell"})) != 1 {
		t.Fatal("host handshake capabilities changed")
	}
	for _, role := range []string{"bastion", "kubernetes"} {
		existing := []string{"kubernetes.query"}
		got := interactiveCapabilities(role, existing)
		if len(got) != 3 || len(existing) != 1 || len(interactiveCapabilities(role, got)) != 3 {
			t.Fatal("native capabilities mutated enrollment identity or duplicated")
		}
	}
	target := kubernetesstream.Target{SchemaVersion: kubernetesstream.SchemaVersion, APIServer: "https://cluster.example.test", Mode: "exec", Shell: "sh", Pin: kubernetespodlogs.Pin{Namespace: "orders", NamespaceUID: "ns-uid", Pod: "api", PodUID: "pod-uid", Container: "main", ContainerID: "containerd://one"}, SecurityFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	raw, _ := json.Marshal(target)
	open := &connectorv1.RemoteAccessOpen{Protocol: "kubernetes_exec", SessionFence: 1, MaxFrameBytes: kubernetesstream.MaxFrameBytes, TerminalCols: 100, TerminalRows: 30, KubernetesTargetJson: raw}
	if _, e := newLocalRemoteSession(open); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"shell", "ssh", "rdp", "kubernetes_port_forward"} {
		copy := *open
		copy.Protocol = mode
		if _, e := newLocalRemoteSession(&copy); e == nil {
			t.Fatal("cross-protocol target accepted", mode)
		}
	}
	for _, bad := range [][]byte{nil, []byte(`{}`), []byte(`{"schema_version":"argus.kubernetes_stream/v2"}`), make([]byte, 17<<10)} {
		copy := *open
		copy.KubernetesTargetJson = bad
		if _, e := newLocalRemoteSession(&copy); e == nil {
			t.Fatal("unbound target accepted")
		}
	}
}
