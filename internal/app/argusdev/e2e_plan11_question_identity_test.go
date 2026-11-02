package argusdev

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestQuestionAbbreviationsNeedUniqueObservedCandidateIdentities(t *testing.T) {
	first := uuid.MustParse("661527fa-150f-5a11-9a23-f44d9b0b0db5")
	second := uuid.MustParse("bc72a826-afed-58c6-948c-bf6c2a7563ab")
	candidates := []uuid.UUID{first, second}
	for _, text := range []string{"661527fa-…0db5", "661527fa...0db5", "661527FA-150F-…0DB5", "661527fa…", "ID 661527fa"} {
		if !plan11QuestionIdentifies(text, first, nil, candidates...) || plan11QuestionIdentifies(text, second, nil, candidates...) {
			t.Fatal("unique actual candidate abbreviation failed or matched another object", text)
		}
	}
	for _, text := range []string{"6615…0db5", "10.77.0.8:3306", "661527f", "661527fa…ffff", "x661527fa…0db5", "661527fa…0db5z"} {
		if plan11QuestionIdentifies(text, first, nil, candidates...) {
			t.Fatal("short, common, embedded or incorrect text resolved an identity", text)
		}
	}
	collision := uuid.MustParse("661527fa-9999-5999-8999-000000000db5")
	if plan11QuestionIdentifies("661527fa-…0db5", first, nil, first, collision) {
		t.Fatal("colliding prefix and suffix were silently treated as unique")
	}
	if plan11QuestionIdentifies("ID 661527fa", first, nil, first, collision) {
		t.Fatal("colliding standalone prefix was silently treated as unique")
	}
	if plan11QuestionIdentifies("661527fa-…0db5", first, nil) || plan11QuestionIdentifies("661527fa-…0db5", first, nil, second) {
		t.Fatal("unobserved candidate universe was invented")
	}
}

func TestOriginalQuestionToolArgumentsReplayUniqueAbbreviations(t *testing.T) {
	path := os.Getenv("ARGUS_PLAN11_QUESTION_REPLAY")
	if path == "" {
		t.Skip("optional sealed original real tool-argument replay")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var calls []struct {
		Tool  string
		Input json.RawMessage
	}
	if json.Unmarshal(raw, &calls) != nil {
		t.Fatal("original real tool trace invalid")
	}
	first := uuid.MustParse("661527fa-150f-5a11-9a23-f44d9b0b0db5")
	second := uuid.MustParse("bc72a826-afed-58c6-948c-bf6c2a7563ab")
	questions := 0
	for _, call := range calls {
		if call.Tool != "request_user_input" {
			continue
		}
		questions++
		if !plan11QuestionIdentifies(string(call.Input), first, nil, first, second) || !plan11QuestionIdentifies(string(call.Input), second, nil, first, second) {
			t.Fatal("actual original question still rejects its observed unique labels")
		}
	}
	if questions != 1 {
		t.Fatal("original cohort did not contain its single actual durable question")
	}
}

func TestOriginalQuestionAbbreviationsDistinguishBothObservedTargets(t *testing.T) {
	first := uuid.MustParse("661527fa-150f-5a11-9a23-f44d9b0b0db5")
	second := uuid.MustParse("bc72a826-afed-58c6-948c-bf6c2a7563ab")
	content := `{"options":[{"label":"661527fa-…0db5"},{"label":"bc72a826-…63ab"}]}`
	for _, target := range []uuid.UUID{first, second} {
		if !plan11QuestionIdentifies(content, target, nil, first, second) {
			t.Fatal("original real question's unique labels were rejected", target)
		}
	}
}
