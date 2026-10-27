package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"strings"
	"testing"
)

func TestMutationDiffKeepsChangedPathsAndRejectsTruncation(t *testing.T) {
	before := &kubernetesinventory.Object{Data: json.RawMessage(`{"metadata":{"name":"settings","uid":"old","resourceVersion":"4","annotations":{"argus.io/operation-id":"private"}},"data":{"mode":"before"}}`)}
	after := &kubernetesinventory.Object{Data: json.RawMessage(`{"metadata":{"name":"settings","uid":"old","resourceVersion":"5","annotations":{"argus.io/operation-id":"private"}},"data":{"mode":"after"}}`)}
	lines, err := PublicMutationDiff(kubernetesinventory.MutationResult{Before: before, After: after})
	if err != nil || len(lines) != 4 {
		t.Fatalf("diff: %+v %v", lines, err)
	}
	raw, _ := json.Marshal(lines)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "resourceVersion") || !strings.Contains(string(raw), "/data/mode") {
		t.Fatalf("wrong public diff: %s", raw)
	}
	value, _ := json.Marshal(map[string]string{"value": strings.Repeat("x", 2049)})
	after.Data = value
	if _, err = PublicMutationDiff(kubernetesinventory.MutationResult{After: after}); err == nil {
		t.Fatal("oversized diff silently truncated or exposed invalid contract")
	}
}
