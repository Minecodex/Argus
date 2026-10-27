package argusdev

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestRealModelConfigurationFreezesExplicitReasoningWithoutSecretOrDefaultOverride(t *testing.T) {
	t.Setenv("ARGUS_BENCHMARK_REASONING_KEY", "reasoning-profile-fixture-key")
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "modelapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{"chat_completions", "responses"} {
		for _, effort := range []string{"", "low"} {
			t.Run(protocol+"/"+effort, func(t *testing.T) {
				config := map[string]any{"base_url": "https://model.example/v1", "model_id": "test-model", "api_protocol": protocol, "api_key_env": "ARGUS_BENCHMARK_REASONING_KEY", "context_window_tokens": 32768, "max_output_tokens": 8192, "input_price_per_million": 0.1, "output_price_per_million": 0.2, "monthly_amount": 5}
				if effort != "" {
					config["reasoning_effort"] = effort
				}
				raw, _ := json.Marshal(config)
				path := filepath.Join(t.TempDir(), "model.json")
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				loaded, err := loadP5RealModelConfig(path)
				if protocol == "responses" && effort != "" {
					if !errors.Is(err, errUsage) {
						t.Fatal("unsupported protocol reasoning must retain the existing fail-closed contract")
					}
					return
				}
				if err != nil {
					t.Fatal("valid optional reasoning profile must be accepted without changing other settings", err)
				}
				request := loaded.modelRequest()
				if effort == "" {
					if _, ok := request["reasoning_effort"]; ok {
						t.Fatal("omitted reasoning must retain provider defaults")
					}
				} else if request["reasoning_effort"] != effort {
					t.Fatal("explicit reasoning profile must reach the normal model creation API")
				}
				if request["max_output_tokens"] != 8192 || request["context_window_tokens"] != 32768 {
					t.Fatal("reasoning choice must not alter frozen context/output budgets")
				}
				if err := document.Components.Schemas["AIModelTestCreate"].Value.VisitJSON(request); err != nil {
					t.Fatal("profile must use the existing canonical model API", err)
				}
				serialized, _ := json.Marshal(loaded)
				var saved map[string]any
				if json.Unmarshal(serialized, &saved) != nil {
					t.Fatal("configuration must remain serializable")
				}
				if _, exists := saved["api_key"]; exists {
					t.Fatal("resolved environment secret must never become configuration")
				}
			})
		}
	}
	raw := []byte(`{"base_url":"https://model.example/v1","model_id":"test-model","api_protocol":"chat_completions","api_key_env":"ARGUS_BENCHMARK_REASONING_KEY","context_window_tokens":32768,"max_output_tokens":8192,"monthly_amount":5,"reasoning_effort":"not-a-valid-profile"}`)
	path := filepath.Join(t.TempDir(), "invalid-profile.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadP5RealModelConfig(path); !errors.Is(err, errUsage) {
		t.Fatal("unknown reasoning profile must fail closed")
	}
}
