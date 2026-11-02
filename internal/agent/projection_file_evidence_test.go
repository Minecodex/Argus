package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestQueryFileChecksumAndPartialReasonsMatchCanonicalProjectionSchema(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	for _, path := range []string{"agent/agent.schema.json", "common/public-json.schema.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "api", "schemas", filepath.FromSlash(path)))
		var document map[string]any
		if err != nil || json.Unmarshal(raw, &document) != nil {
			t.Fatal("canonical schema unavailable", path, err)
		}
		if err = compiler.AddResource(document["$id"].(string), document); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("https://argus.io/schemas/v1/agent/agent.schema.json#/$defs/ToolResultProjection")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		data   map[string]any
		marked bool
		want   []string
	}{
		{"complete", map[string]any{"status": "success"}, false, nil},
		{"tool_partial", map[string]any{"status": "success"}, true, []string{"tool_result_partial"}},
		{"projected_text", map[string]any{"content": strings.Repeat("sample", maxProjectedText)}, false, []string{"budget_truncated"}},
		{"both_origins", map[string]any{"content": strings.Repeat("sample", maxProjectedText)}, true, []string{"budget_truncated", "tool_result_partial"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full, _ := json.Marshal(tc.data)
			encoded, partial, err := encodeToolResultProjection("result_query", "query", full, toolruntime.Result{Data: tc.data, QueryFile: "queries/metrics/query.json", Partial: tc.marked})
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if json.Unmarshal(encoded, &document) != nil {
				t.Fatal("invalid projection")
			}
			if err = schema.Validate(document); err != nil {
				t.Fatal("actual model projection violates canonical contract", err)
			}
			file := document["workspace_file"].(map[string]any)
			if file["sha256"] != fmt.Sprintf("%x", sha256.Sum256(full)) || file["source_result_ref"] != "result_query" || file["path"] != "/workspace/queries/metrics/query.json" {
				t.Fatal("file checksum is not bound to exact original bytes/source/path")
			}
			var reasons []string
			if list, ok := document["partial_reasons"].([]any); ok {
				for _, reason := range list {
					reasons = append(reasons, reason.(string))
				}
			}
			if !slices.Equal(reasons, tc.want) || partial != (len(tc.want) > 0) {
				t.Fatal("partial reason origin changed", reasons)
			}
			readBytes := []byte(`{"content":"source lines","partial":false}`)
			readProjection, _, err := encodeToolResultProjection("result_read", "read", readBytes, toolruntime.Result{Data: map[string]any{"content": "source lines"}})
			if err != nil {
				t.Fatal(err)
			}
			var read map[string]any
			_ = json.Unmarshal(readProjection, &read)
			if read["result_hash"] == file["sha256"] || read["workspace_file"] != nil {
				t.Fatal("read envelope hash was presented as the original file checksum")
			}
		})
	}
}
