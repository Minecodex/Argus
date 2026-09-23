package argusdev

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// The configuration contains only a reference to a process environment secret.
// Neither the config nor benchmark evidence carries the resolved API key.
type p5RealModelConfig struct {
	BaseURL       string  `json:"base_url"`
	ModelID       string  `json:"model_id"`
	Protocol      string  `json:"api_protocol"`
	APIKeyEnv     string  `json:"api_key_env"`
	ContextWindow int     `json:"context_window_tokens"`
	MaxOutput     int     `json:"max_output_tokens"`
	InputPrice    float64 `json:"input_price_per_million"`
	OutputPrice   float64 `json:"output_price_per_million"`
	MonthlyAmount float64 `json:"monthly_amount"`
}

func loadP5RealModelConfig(path string) (*p5RealModelConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read real-model configuration: %v", errUsage, err)
	}
	defer file.Close()
	var config p5RealModelConfig
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		// Do not echo decoder errors: unknown field names may contain credentials.
		return nil, fmt.Errorf("%w: invalid real-model JSON configuration", errUsage)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("%w: real-model configuration must contain one JSON object", errUsage)
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c p5RealModelConfig) validate() error {
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil || len(c.BaseURL) > 2048 || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("%w: real-model base_url must be an HTTPS endpoint without credentials, query or fragment", errUsage)
	}
	if strings.Contains(strings.ToLower(c.BaseURL+" "+c.ModelID), "argus-replay") {
		return fmt.Errorf("%w: Replay cannot be used as a real-model benchmark", errUsage)
	}
	if strings.TrimSpace(c.ModelID) == "" || len(c.ModelID) > 256 || (c.Protocol != "chat_completions" && c.Protocol != "responses") {
		return fmt.Errorf("%w: real-model model_id or api_protocol is invalid", errUsage)
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(c.APIKeyEnv) || strings.TrimSpace(os.Getenv(c.APIKeyEnv)) == "" {
		return fmt.Errorf("%w: real-model api_key_env must reference a populated environment variable", errUsage)
	}
	if c.ContextWindow < 8192 || c.MaxOutput < 1 || c.MaxOutput >= c.ContextWindow || c.InputPrice < 0 || c.OutputPrice < 0 || c.MonthlyAmount <= 0 {
		return fmt.Errorf("%w: real-model context, output or quota configuration is invalid", errUsage)
	}
	return nil
}

func (c p5RealModelConfig) modelRequest() map[string]any {
	return map[string]any{"name": "P5 real-model benchmark", "base_url": c.BaseURL, "model_id": c.ModelID,
		"api_protocol": c.Protocol, "api_key": os.Getenv(c.APIKeyEnv), "context_window_tokens": c.ContextWindow,
		"max_output_tokens": c.MaxOutput, "input_price_per_million": c.InputPrice, "output_price_per_million": c.OutputPrice}
}
