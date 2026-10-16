package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestToolResultProjectionBoundsListsAndTracksPartialState(t *testing.T) {
	t.Parallel()
	items := make([]map[string]any, 0, 80)
	for index := range 80 {
		items = append(items, map[string]any{"id": index, "name": strings.Repeat("host", 20)})
	}
	full, _ := json.Marshal(map[string]any{"items": items})
	encoded, partial, err := encodeToolResultProjection("result_1", "01900000-0000-7000-8000-000000000001", full,
		toolruntime.Result{Data: map[string]any{"items": items}})
	if err != nil {
		t.Fatal(err)
	}
	if !partial || len(encoded) > maxProjectionBytes {
		t.Fatalf("partial=%t bytes=%d", partial, len(encoded))
	}
	var projection map[string]any
	if json.Unmarshal(encoded, &projection) != nil {
		t.Fatal("projection is not JSON")
	}
	if int(projection["projected_bytes"].(float64)) != len(encoded) || projection["partial"] != true {
		t.Fatalf("projection metadata is inconsistent: %#v", projection)
	}
	if projection["tool_call_id"] != "01900000-0000-7000-8000-000000000001" {
		t.Fatalf("projection must expose the persisted ToolCall id: %#v", projection["tool_call_id"])
	}
	summary := projection["summary"].(map[string]any)
	if len(summary["items"].([]any)) != maxProjectedItems {
		t.Fatalf("projected items = %d", len(summary["items"].([]any)))
	}
}

func TestCompleteDiscoveryMetadataSurvivesModelProjection(t *testing.T) {
	items := make([]map[string]any, 0, 300)
	for i := range 300 {
		items = append(items, map[string]any{"name": fmt.Sprintf("inspect_%04d", i), "summary": strings.Repeat("用途", 120)})
	}
	data := map[string]any{"category": "host", "items": items, "complete": true, "api_key": "private"}
	full, _ := json.Marshal(data)
	encoded, partial, err := encodeToolResultProjection("catalog", "call", full, toolruntime.Result{Data: data, DataIsDiscovery: true})
	if err != nil || partial || len(encoded) <= maxProjectionBytes || strings.Contains(string(encoded), "private") {
		t.Fatal("complete discovery metadata was clipped or bypassed sanitization", err)
	}
	var projection map[string]any
	if err = json.Unmarshal(encoded, &projection); err != nil {
		t.Fatal(err)
	}
	summary := projection["summary"].(map[string]any)
	if len(summary["items"].([]any)) != 300 || summary["complete"] != true {
		t.Fatal("model did not receive the full category list")
	}
}

func TestDomainModelViewRetainsCompleteArtifactHashAndSanitization(t *testing.T) {
	data := map[string]any{"complete": strings.Repeat("history", 10000)}
	full, _ := json.Marshal(data)
	view := map[string]any{"resource_ids": []any{"host"}, "value": 12.5, "api_key": "private"}
	encoded, _, err := encodeToolResultProjection("result_full", "call", full, toolruntime.Result{Data: data, ModelSummary: view})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(encoded, &got)
	if got["result_hash"] != fmt.Sprintf("%x", sha256.Sum256(full)) || got["original_bytes"] != float64(len(full)) || got["result_ref"] != "result_full" || strings.Contains(string(encoded), "private") || got["summary"].(map[string]any)["value"] != float64(12.5) {
		t.Fatal("domain view lost artifact integrity or leaked sensitive fields")
	}
}

func TestTextProjectionRedactsAndTruncates(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("line\n", 10_000)
	structured := map[string]any{"cluster_id": "cluster", "namespace": "default", "pod": "pod", "content": content, "bytes": len(content), "api_key": "argus_ak_prefix.secret"}
	full, _ := json.Marshal(structured)
	encoded, partial, err := encodeToolResultProjection("result_logs", "01900000-0000-7000-8000-000000000002", full, toolruntime.Result{Data: structured})
	if err != nil {
		t.Fatal(err)
	}
	if !partial || len(encoded) > maxProjectionBytes || strings.Contains(string(encoded), "argus_ak_") {
		t.Fatalf("unsafe log projection: partial=%t bytes=%d", partial, len(encoded))
	}
}
