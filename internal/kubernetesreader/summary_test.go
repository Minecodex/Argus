package kubernetesreader

import (
	"encoding/json"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
	"testing"
)

func TestResourceSummaryCarriesActualReadyStateWithoutSpecSecrets(t *testing.T) {
	object := unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "observed", "annotations": map[string]any{"private": "do-not-project"}}, "spec": map[string]any{"env": []any{"do-not-project"}}, "status": map[string]any{"desiredNumberScheduled": int64(4), "numberReady": int64(1)}}}
	summary := resourceSummary("daemonset", object)
	status := summary["status"].(map[string]any)
	if status["numberReady"] != int64(1) || status["desiredNumberScheduled"] != int64(4) {
		t.Fatalf("missing actual counts: %+v", summary)
	}
	raw, _ := json.Marshal(summary)
	if strings.Contains(string(raw), "do-not-project") {
		t.Fatal("private metadata or spec leaked")
	}
}
func TestConnectorAndDirectResourceProjectionShareObservedStatus(t *testing.T) {
	object, err := objectFromJSON([]byte(`{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"collector","labels":{"argus.io/collector-id":"observed"}},"status":{"desiredNumberScheduled":3,"numberReady":2}}`), "daemonset")
	if err != nil {
		t.Fatal(err)
	}
	status, ok := object.Summary["status"].(map[string]any)
	if !ok || status["numberReady"] != int64(2) {
		t.Fatalf("Connector lost workload status: %+v", object)
	}
}
