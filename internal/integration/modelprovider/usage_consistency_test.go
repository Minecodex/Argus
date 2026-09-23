package modelprovider

import (
	"errors"
	"strings"
	"testing"
)

func TestFinalUsageConsistencyAcrossUpdates(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		updates               []string
		input, output, cached int64
		invalid, cacheMissing bool
	}{
		{"input regression", []string{`{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":80}}`, `{"prompt_tokens":50,"completion_tokens":8}`}, 50, 8, 80, true, false},
		{"cache first", []string{`{"prompt_tokens_details":{"cached_tokens":80}}`, `{"prompt_tokens":100,"completion_tokens":8}`}, 100, 8, 80, false, false},
		{"revised cache", []string{`{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":80}}`, `{"prompt_tokens":50,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":20}}`}, 50, 8, 20, false, false},
		{"resolved intermediate", []string{`{"prompt_tokens":50,"completion_tokens":8}`, `{"prompt_tokens_details":{"cached_tokens":80}}`, `{"prompt_tokens":100}`}, 100, 8, 80, false, false},
		{"repeated", []string{`{"prompt_tokens":100,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":80}}`, `{"prompt_tokens":100,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":80}}`}, 100, 8, 80, false, false},
		{"output only", []string{`{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":80}}`, `{"completion_tokens":8}`}, 100, 8, 80, false, false},
		{"orphan cache", []string{`{"prompt_tokens_details":{"cached_tokens":80}}`, `{"completion_tokens":8}`}, 0, 8, 80, true, false},
		{"zero", []string{`{"prompt_tokens_details":{"cached_tokens":0}}`, `{"prompt_tokens":0,"completion_tokens":0}`}, 0, 0, 0, false, false},
		{"missing cache", []string{`{"prompt_tokens":100,"completion_tokens":8}`}, 100, 8, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ending := range []string{"complete", "failed"} {
				var stream strings.Builder
				for _, update := range tc.updates {
					stream.WriteString("data: {\"choices\":[],\"usage\":" + update + "}\n\n")
				}
				if ending == "complete" {
					stream.WriteString("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
				var usage TokenUsage
				err := (Provider{Protocol: ProtocolChatCompletions}).consumeSSE(strings.NewReader(stream.String()), func(e Event) error { usage.Observe(e); return nil })
				if errors.Is(err, ErrInvalidUsage) != tc.invalid || ending == "complete" && !tc.invalid && err != nil {
					t.Fatalf("%s error=%v", ending, err)
				}
				if usage.Input != tc.input || usage.Output != tc.output || usage.CachedInput != tc.cached {
					t.Fatalf("usage=%+v", usage)
				}
				if usage.Complete() == tc.invalid || usage.CacheComplete() != (!tc.invalid && !tc.cacheMissing) {
					t.Fatalf("completeness=%v/%v", usage.Complete(), usage.CacheComplete())
				}
				if tc.invalid {
					usage.Estimate(1000, 1000)
					if usage.InputSource() != "invalid" || usage.OutputSource() != "invalid" || usage.CachedSource() != "invalid" || usage.Input != tc.input {
						t.Fatal("invalid usage masked")
					}
				}
			}
		})
	}
}

func TestRejectedUsageRemainsInvalidAfterEstimation(t *testing.T) {
	var usage TokenUsage
	usage.Observe(Event{Input: 100, InputKnown: true, Output: 8, OutputKnown: true})
	usage.Invalidate()
	usage.Estimate(200, 20)
	if !errors.Is(usage.Validate(), ErrInvalidUsage) || usage.Complete() || usage.CacheComplete() || usage.InputSource() != "invalid" || usage.OutputSource() != "invalid" || usage.CachedSource() != "invalid" || usage.Input != 100 || usage.Output != 8 {
		t.Fatalf("rejected usage=%+v", usage)
	}
}
