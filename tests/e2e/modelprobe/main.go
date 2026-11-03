// modelprobe exercises the production provider transport in an owned container.
// It records only completion status, token usage and latency, never credentials.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
)

func main() {
	count := flag.Int("count", 6, "bounded number of sequential compatibility requests")
	output := flag.String("output", "", "JSON evidence path")
	flag.Parse()
	if *count < 1 || *count > 20 || *output == "" || os.Getenv("ARGUS_PLAN11_API_KEY") == "" {
		os.Exit(2)
	}
	policy := modelprovider.PublicEndpointPolicy{}
	base := "https://api.apimart.ai/v1"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := policy.Validate(ctx, base); err != nil {
		fmt.Fprintln(os.Stderr, "provider endpoint validation failed")
		os.Exit(1)
	}
	provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: base, APIKey: os.Getenv("ARGUS_PLAN11_API_KEY"), Client: policy.ModelClient()}
	rows := []map[string]any{}
	passed := 0
	for i := 0; i < *count; i++ {
		started := time.Now()
		usage := modelprovider.TokenUsage{}
		text, complete := false, false
		err := provider.Stream(ctx, modelprovider.Request{Model: "glm-5.3-flash", Messages: []modelprovider.Message{{Role: "user", Content: "Reply with exactly OK."}}, MaxTokens: 1024, ReasoningEffort: "low"}, func(event modelprovider.Event) error {
			usage.Observe(event)
			if event.Type == "text_delta" && event.Text != "" {
				text = true
			}
			if event.Type == "completed" {
				complete = true
			}
			return nil
		})
		ok := err == nil && text && complete && usage.Complete()
		if ok {
			passed++
		}
		row := map[string]any{"index": i + 1, "passed": ok, "completed": complete, "text_received": text, "usage": usage, "elapsed_ms": time.Since(started).Milliseconds()}
		if err != nil {
			row["error_type"] = fmt.Sprintf("%T", err)
		}
		rows = append(rows, row)
	}
	raw, _ := json.MarshalIndent(map[string]any{"model": "glm-5.3-flash", "godebug": os.Getenv("GODEBUG"), "passed": passed == *count, "successful_calls": passed, "calls": rows}, "", "  ")
	if err := os.WriteFile(*output, raw, 0600); err != nil {
		os.Exit(1)
	}
	fmt.Printf("provider_probe successful=%d total=%d\n", passed, *count)
	if passed != *count {
		os.Exit(1)
	}
}
