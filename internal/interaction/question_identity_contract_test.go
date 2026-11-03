package interaction

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestQuestionIDGuidanceKeepsStrictDurableIdentity(t *testing.T) {
	schema := QuestionSchema()
	question := schema["properties"].(map[string]any)["questions"].(map[string]any)["items"].(map[string]any)
	id := question["properties"].(map[string]any)["id"].(map[string]any)
	if !strings.Contains(id["description"].(string), "EVERY question") || id["examples"].([]string)[0] != "target_pick" {
		t.Fatal("required stable question identity guidance missing")
	}
	compiled, err := toolruntime.CompileInputSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"header": "Target", "question": "Which exact target?", "options": []any{}}
	if compiled.Validate(map[string]any{"questions": []any{input}}) == nil {
		t.Fatal("missing question identity was silently accepted")
	}
	input["id"] = "target_pick"
	if err := compiled.Validate(map[string]any{"questions": []any{input}}); err != nil {
		t.Fatal("described stable identity was rejected", err)
	}
}
