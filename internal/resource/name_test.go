package resource

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeResourceName(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
		invalid           bool
	}{
		{name: "trim whitespace", input: " \t 主机 Alpha \u3000", want: "主机 Alpha"},
		{name: "preserve internal spaces and case", input: "Host  Alpha", want: "Host  Alpha"},
		{name: "Unicode length boundary", input: strings.Repeat("主", 128), want: strings.Repeat("主", 128)},
		{name: "empty", invalid: true},
		{name: "whitespace only", input: " \t\n\u3000", invalid: true},
		{name: "ASCII too long", input: strings.Repeat("a", 129), invalid: true},
		{name: "Unicode too long", input: strings.Repeat("主", 129), invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeResourceName(test.input)
			if test.invalid {
				if !errors.Is(err, ErrInvalidResourceName) {
					t.Fatalf("invalid resource name error = %v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("normalized name = %q, error %v; want %q", got, err, test.want)
			}
		})
	}
}
