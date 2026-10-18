package workspace

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type resultsModelResolver struct {
	host      string
	addresses []netip.Addr
}

func (r resultsModelResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	if host == r.host {
		return r.addresses, nil
	}
	return net.DefaultResolver.LookupNetIP(ctx, network, host)
}

// Opt-in acceptance uses real provider tool calls and the same file RPC that
// delivered the business evidence. No credential or full prompt is persisted.
func TestResultsRealModelFindsEvidenceAndCreatesBusinessReport(t *testing.T) {
	if os.Getenv("ARGUS_RESULTS_REAL_MODEL") != "1" {
		t.Skip("requires explicit real-model acceptance")
	}
	key := os.Getenv("GLM_API_KEY")
	if key == "" {
		t.Fatal("GLM_API_KEY is required")
	}
	configuration, err := os.ReadFile("../../docs/plan6/e2e/glm-model.json")
	if err != nil {
		t.Fatal(err)
	}
	var selected struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model_id"`
	}
	if json.Unmarshal(configuration, &selected) != nil || selected.BaseURL == "" || selected.Model == "" {
		t.Fatal("invalid configured Coding Plan model")
	}
	f := newResultsFixture(t)
	nonce := uuid.NewString()
	before := `{"error_count":7,"evidence_nonce":"` + nonce + `"}`
	after := `{"error_count":2}`
	for _, item := range []struct{ path, body string }{{"nginx/before/status.json", before}, {"nginx/after/status.json", after}} {
		if _, err := f.transfer(resultArtifact(item.path, item.body), item.body); err != nil {
			t.Fatal(err)
		}
	}
	property := func(required []string, fields map[string]any) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": fields}
	}
	stringProperty := map[string]any{"type": "string"}
	tools := []modelprovider.Tool{
		{Name: "read", Description: "Read the current UTF-8 text file under /workspace/results through the Workspace file RPC.", Schema: property([]string{"path"}, map[string]any{"path": stringProperty})},
		{Name: "write", Description: "Create a UTF-8 business report under /workspace/results through the Workspace file RPC.", Schema: property([]string{"path", "content"}, map[string]any{"path": stringProperty, "content": stringProperty})},
	}
	messages := []modelprovider.Message{
		{Role: "system", Content: "Use the provided tools to perform the requested evidence analysis. Never invent file contents or IDs." + toolruntime.WorkspaceResultsInstructions},
		{Role: "user", Content: "Read /workspace/results/meta.json to find nginx before/after error evidence. Read both evidence files. Create /workspace/results/nginx/analysis/comparison.json containing numeric before_errors, after_errors, reduction (= before - after), and evidence_nonce copied exactly from the before evidence. Use only actual file bytes. Finish after writing the verified report. The input files are not attached to this prompt."},
	}
	policy := modelprovider.PublicEndpointPolicy{}
	// An opt-in public-DNS override keeps this test usable on development hosts
	// whose VPN returns 198.18/15 Fake-IP. The production public-address policy,
	// hostname TLS verification and redirect checks still apply to every dial.
	if configured := os.Getenv("ARGUS_RESULTS_MODEL_RESOLVER_IPS"); configured != "" {
		endpoint, err := url.Parse(selected.BaseURL)
		if err != nil {
			t.Fatal(err)
		}
		resolver := resultsModelResolver{host: endpoint.Hostname()}
		for _, address := range strings.Split(configured, ",") {
			parsed, err := netip.ParseAddr(strings.TrimSpace(address))
			if err != nil {
				t.Fatal(err)
			}
			resolver.addresses = append(resolver.addresses, parsed)
		}
		policy.Resolver = resolver
		t.Log("public DNS override selected; standard public-address and HTTPS validation retained")
	}
	provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: selected.BaseURL, APIKey: key, Client: policy.ModelClient(), StreamTimeout: 2 * time.Minute}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	reads := map[string]bool{}
	calls, inputTokens, outputTokens := 0, int64(0), int64(0)
	for round := 0; round < 6; round++ {
		var usage modelprovider.TokenUsage
		var answer strings.Builder
		toolCalls := []modelprovider.ToolCall{}
		err := provider.Stream(ctx, modelprovider.Request{Model: selected.Model, Messages: messages, Tools: tools, MaxTokens: 4096, OutputTokenParameter: "max_tokens", ReasoningEffort: "low"}, func(event modelprovider.Event) error {
			usage.Observe(event)
			if event.Type == "text_delta" {
				answer.WriteString(event.Text)
			}
			if event.Type == "tool_call_done" {
				toolCalls = append(toolCalls, modelprovider.ToolCall{ID: event.ToolCallID, Name: event.ToolName, Arguments: event.Arguments})
			}
			return nil
		})
		calls++
		if err != nil {
			t.Fatalf("real provider call %d failed: %v", calls, err)
		}
		if !usage.Complete() {
			t.Fatal("real provider usage is unavailable or inconsistent")
		}
		inputTokens += usage.Input
		outputTokens += usage.Output
		messages = append(messages, modelprovider.Message{Role: "assistant", Content: answer.String(), ToolCalls: toolCalls})
		if len(toolCalls) == 0 {
			break
		}
		for _, call := range toolCalls {
			var arguments struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(call.Arguments), &arguments) != nil {
				t.Fatal("model emitted invalid tool arguments")
			}
			relative := strings.TrimPrefix(arguments.Path, "/workspace/")
			if !strings.HasPrefix(relative, "results/") || path.Clean(relative) != relative || strings.Contains(relative, "..") {
				t.Fatal("model left results scope")
			}
			result := ""
			switch call.Name {
			case "read":
				reader, _, err := f.client.Read(ctx, relative, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(io.LimitReader(reader, 64<<10))
				_ = reader.Close()
				if err != nil {
					t.Fatal(err)
				}
				reads[relative] = true
				result = string(data)
			case "write":
				if relative != "results/nginx/analysis/comparison.json" {
					t.Fatal("model wrote an unrequested result")
				}
				if _, err := f.client.Write(ctx, relative, strings.NewReader(arguments.Content), int64(len(arguments.Content)), false); err != nil {
					t.Fatal(err)
				}
				result = `{"written":true}`
			default:
				t.Fatal("unexpected model tool")
			}
			messages = append(messages, modelprovider.Message{Role: "tool", ToolCallID: call.ID, Content: result})
		}
	}
	for _, required := range []string{"results/meta.json", "results/nginx/before/status.json", "results/nginx/after/status.json"} {
		if !reads[required] {
			t.Fatalf("model skipped evidence %s", required)
		}
	}
	var report struct {
		Before    int    `json:"before_errors"`
		After     int    `json:"after_errors"`
		Reduction int    `json:"reduction"`
		Nonce     string `json:"evidence_nonce"`
	}
	if json.Unmarshal([]byte(f.read("results/nginx/analysis/comparison.json")), &report) != nil || report.Before != 7 || report.After != 2 || report.Reduction != 5 || report.Nonce != nonce {
		t.Fatalf("model report did not use actual evidence: %+v", report)
	}
	t.Logf("real model=%s calls=%d input_tokens=%d output_tokens=%d; metadata + both evidence files read; business comparison report verified", selected.Model, calls, inputTokens, outputTokens)
}
