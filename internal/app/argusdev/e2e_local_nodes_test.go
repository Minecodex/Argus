package argusdev

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestLocalBridgeUsesReadyWorkerAndKeepsDesktopScope(t *testing.T) {
	ready := func(name string, control bool) corev1.Node {
		labels := map[string]string{}
		if control {
			labels["node-role.kubernetes.io/control-plane"] = ""
		}
		return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-generation"), Labels: labels}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	}
	nodes := []corev1.Node{ready("plan8-owned-control-plane", true), ready("plan8-owned-worker", false)}
	selected, err := localBridgeNodeCandidates("kind-plan8-owned", nodes)
	if err != nil || len(selected) != 2 || selected[0].Name != "plan8-owned-worker" {
		t.Fatal("owned multi-node bridge did not prefer the ready worker", err)
	}
	if _, err = localBridgeNodeCandidates("docker-desktop", nodes); err == nil {
		t.Fatal("multi-node input escaped the existing Desktop restriction")
	}
	if _, err = localBridgeNodeCandidates("kind-customer-production", nodes); err == nil {
		t.Fatal("another context was admitted as a local fixture")
	}
	desktop := []corev1.Node{ready("desktop-control-plane", true)}
	if selected, err = localBridgeNodeCandidates("docker-desktop", desktop); err != nil || len(selected) != 1 {
		t.Fatal("approved Desktop scope rejected", err)
	}
	for i := range nodes {
		nodes[i].Status.Conditions[0].Status = corev1.ConditionFalse
	}
	if _, err = localBridgeNodeCandidates("kind-plan8-owned", nodes); err == nil {
		t.Fatal("unready nodes admitted")
	}
}

func TestLocalKindReceiptRequiresExactContainerAndSharedNodeAddress(t *testing.T) {
	root := t.TempDir()
	const cluster, node = "plan8-owned", "plan8-owned-worker"
	actual := localE2ENode{ID: strings.Repeat("a", 64)}
	actual.Config.Labels = map[string]string{"io.x-k8s.kind.cluster": cluster}
	actual.NetworkSettings.Networks = map[string]localE2ENetwork{"shared": {NetworkID: "shared-kind-network", IPAddress: "172.18.0.3"}}
	receipt := localKindReceipt{Schema: "argus.plan8_network_owner/v1", RunID: cluster, Workspace: root}
	receipt.Nodes = append(receipt.Nodes, struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}{node, actual.ID})
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLocalKindReceipt(append([]byte{0xef, 0xbb, 0xbf}, raw...))
	if err != nil || decoded.RunID != receipt.RunID || decoded.Workspace != receipt.Workspace {
		t.Fatal("valid PowerShell UTF-8 BOM receipt did not retain its identity", err)
	}
	if err := validateOwnedKindNode(root, "kind-"+cluster, node, actual, receipt); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"root", "cluster", "container", "missing", "duplicate", "runtime_owner"} {
		copy := receipt
		copy.Nodes = append(copy.Nodes[:0:0], receipt.Nodes...)
		runtime := actual
		switch change {
		case "root":
			copy.Workspace = root + "-another"
		case "cluster":
			copy.RunID = "plan8-another"
		case "container":
			copy.Nodes[0].ID = strings.Repeat("b", 64)
		case "missing":
			copy.Nodes = nil
		case "duplicate":
			copy.Nodes = append(copy.Nodes, copy.Nodes[0])
		case "runtime_owner":
			runtime.Config.Labels = map[string]string{"io.x-k8s.kind.cluster": "plan8-another"}
		}
		if err := validateOwnedKindNode(root, "kind-"+cluster, node, runtime, copy); err == nil {
			t.Fatal("foreign or ambiguous local node ownership admitted", change)
		}
	}
	kubeNode := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: node}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "172.18.0.3"}}}}
	shared, ip, err := localNodeSharedNetwork(kubeNode, actual)
	if err != nil || shared != "shared-kind-network" || ip != "172.18.0.3" {
		t.Fatal("actual shared node network was not retained", err)
	}
	kubeNode.Status.Addresses[0].Address = "172.18.0.4"
	if _, _, err = localNodeSharedNetwork(kubeNode, actual); err == nil {
		t.Fatal("mismatching Docker/Kubernetes node IP admitted")
	}
}
