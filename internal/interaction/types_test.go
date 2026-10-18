package interaction

import "testing"

func TestQuestionAndAnswerValidation(t *testing.T) {
	q := Questions{Questions: []Question{{ID: "scope", Header: "Scope", Question: "Which target?", Options: []Option{{Label: "Owned", Description: "Test only"}, {Label: "Custom", Description: "Explain"}}}}}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []Response{{Action: "answer"}, {Action: "answer", Answers: map[string]Answer{"scope": {SelectedOption: "Forged"}}}, {Action: "answer", Answers: map[string]Answer{"scope": {Skipped: true, Text: "hidden"}}}} {
		if validateAnswers(q, r) == nil {
			t.Fatal("invalid or fabricated answer accepted")
		}
	}
	for _, r := range []Response{{Action: "skip"}, {Action: "answer", Answers: map[string]Answer{"scope": {Text: "Custom target"}}}, {Action: "answer", Answers: map[string]Answer{"scope": {SelectedOption: "Owned", Text: "Only temp files"}}}, {Action: "answer", Answers: map[string]Answer{"scope": {Skipped: true}}}} {
		if err := validateAnswers(q, r); err != nil {
			t.Fatal(err)
		}
	}
	q.Questions = append(q.Questions, q.Questions[0])
	if q.Validate() == nil {
		t.Fatal("duplicate question IDs accepted")
	}
}
