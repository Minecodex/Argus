package modelprovider

import (
	"fmt"
	"strings"
	"testing"
)

func TestCachedInputUsageForBothProtocols(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolChatCompletions, ProtocolResponses} {
		for _, test := range []struct {
			name, details  string
			want           int64
			known, invalid bool
		}{
			{"missing", "", 0, false, false}, {"zero", `,"DETAILS":{"cached_tokens":0}`, 0, true, false},
			{"hit", `,"DETAILS":{"cached_tokens":12}`, 12, true, false},
			{"negative", `,"DETAILS":{"cached_tokens":-1}`, 0, false, true}, {"over_input", `,"DETAILS":{"cached_tokens":33}`, 0, false, true},
		} {
			t.Run(string(protocol)+"/"+test.name, func(t *testing.T) {
				field, details := "prompt_tokens", test.details
				output := "completion_tokens"
				if protocol == ProtocolResponses {
					field, output = "input_tokens", "output_tokens"
				}
				details = strings.ReplaceAll(details, "DETAILS", field+"_details")
				usage := fmt.Sprintf(`{"%s":32,"%s":8%s}`, field, output, details)
				wire := `{"choices":[],"usage":` + usage + `}`
				if protocol == ProtocolResponses {
					wire = `{"type":"response.completed","response":{"status":"completed","usage":` + usage + `}}`
				}
				var value TokenUsage
				err := (Provider{Protocol: protocol}).decodeEvent([]byte(wire), func(e Event) error { value.Observe(e); return nil })
				if (err != nil) != test.invalid {
					t.Fatalf("invalid=%v err=%v", test.invalid, err)
				}
				if !test.invalid && (value.CachedInput != test.want || value.CacheComplete() != test.known || value.Input != 32 || value.Output != 8) {
					t.Fatalf("cache usage=%+v known=%v", value, value.CacheComplete())
				}
			})
		}
	}
}
