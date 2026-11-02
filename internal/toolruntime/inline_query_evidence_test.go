package toolruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompleteInlineQueryOriginalPreservesNumbersAndRejectsUnknown(t *testing.T) {
	for _, c := range []struct {
		original, visible string
		partial, want     bool
	}{
		{`{"n":9007199254740993}`, `{"n":9007199254740993}`, false, true},
		{`{"n":9007199254740993}`, `{"n":9007199254740992}`, false, false},
		{`{"a":1,"b":2}`, `{ "b":2, "a":1 }`, false, true},
		{`{"a":1}`, `{"a":1}`, true, false},
		{`{"a":1} {}`, `{"a":1}`, false, false},
		{`{"a":1}`, `{"a":1} {}`, false, false},
		{`null`, `null`, false, false},
		{`{"text":"` + strings.Repeat("a", MaxInlineQueryOriginalBytes) + `"}`, `{"text":"short"}`, false, false},
	} {
		if CompleteInlineQueryOriginal([]byte(c.original), json.RawMessage(c.visible), c.partial) != c.want {
			t.Fatal("incorrect completeness decision", c.want)
		}
	}
}
