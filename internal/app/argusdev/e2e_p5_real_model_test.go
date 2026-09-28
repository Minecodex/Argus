package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5"
)

func TestP5BenchmarkStatisticsDatabase(t *testing.T) {
	connectionString := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if connectionString == "" {
		t.Skip("ARGUS_P5_TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		t.Fatal("connect to benchmark test database failed")
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, "EXPLAIN "+p5BenchmarkSampleSQL("00000000-0000-0000-0000-000000000000")); err != nil {
		t.Fatal(err)
	}
}

func TestP5RealModelConfigurationAndRequestContract(t *testing.T) {
	t.Setenv("ARGUS_BENCHMARK_TEST_KEY", "fixture-api-key")
	t.Setenv("ARGUS_BENCHMARK_MISSING_KEY", "")
	config := p5RealModelConfig{BaseURL: "https://model.example/v1", ModelID: "test-model", Protocol: "chat_completions", APIKeyEnv: "ARGUS_BENCHMARK_TEST_KEY", ContextWindow: 32768, MaxOutput: 4096, InputPrice: 0.1, OutputPrice: 0.2, MonthlyAmount: 5}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "modelapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{"chat_completions", "responses"} {
		config.Protocol = protocol
		data, _ := json.Marshal(config)
		if strings.Contains(string(data), "fixture-api-key") {
			t.Fatal("configuration exposed secret")
		}
		path := filepath.Join(t.TempDir(), "model.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadP5RealModelConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		request, _ := json.Marshal(loaded.modelRequest())
		var wire map[string]any
		if err := json.Unmarshal(request, &wire); err != nil {
			t.Fatal(err)
		}
		if err := document.Components.Schemas["AIModelTestCreate"].Value.VisitJSON(wire); err != nil {
			t.Fatal(err)
		}
		if wire["api_key"] != "fixture-api-key" {
			t.Fatal("environment credential was not applied")
		}
	}
	for name, modify := range map[string]func(*p5RealModelConfig){
		"http":                func(c *p5RealModelConfig) { c.BaseURL = "http://model.example/v1" },
		"url credentials":     func(c *p5RealModelConfig) { c.BaseURL = "https://user:pass@model.example/v1" },
		"url query":           func(c *p5RealModelConfig) { c.BaseURL = "https://model.example/v1?key=secret" },
		"Replay endpoint":     func(c *p5RealModelConfig) { c.BaseURL = "https://argus-replay-model.test/v1" },
		"Replay model":        func(c *p5RealModelConfig) { c.ModelID = "argus-replay-test" },
		"protocol":            func(c *p5RealModelConfig) { c.Protocol = "unknown" },
		"missing key":         func(c *p5RealModelConfig) { c.APIKeyEnv = "ARGUS_BENCHMARK_MISSING_KEY" },
		"context":             func(c *p5RealModelConfig) { c.ContextWindow = 4096 },
		"output":              func(c *p5RealModelConfig) { c.MaxOutput = c.ContextWindow },
		"quota":               func(c *p5RealModelConfig) { c.MonthlyAmount = 0 },
		"private endpoint IP": func(c *p5RealModelConfig) { c.EndpointIP = "10.0.0.1" },
		"fake endpoint IP":    func(c *p5RealModelConfig) { c.EndpointIP = "198.18.0.80" },
		"invalid endpoint IP": func(c *p5RealModelConfig) { c.EndpointIP = "not-an-ip" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := config
			modify(&invalid)
			if !errors.Is(invalid.validate(), errUsage) {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, data := range []string{`{"api_key":"fixture-api-key"}`, `{"fixture-api-key":"x"}`, `{} {}`, strings.Repeat("x", 16386)} {
		path := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := loadP5RealModelConfig(path)
		if !errors.Is(err, errUsage) || strings.Contains(err.Error(), "fixture-api-key") {
			t.Fatalf("unsafe validation error: %v", err)
		}
	}
}

func TestRealModelEndpointIPIsOnlyADeploymentHint(t *testing.T) {
	config := p5RealModelConfig{BaseURL: "https://model.example/v1", EndpointIP: "8.8.8.8"}
	if err := validateRealModelEndpointIP(config); err != nil {
		t.Fatal(err)
	}
	if _, exists := config.modelRequest()["endpoint_ip"]; exists {
		t.Fatal("deployment DNS hint escaped into product model configuration")
	}
}

func TestP5SalesCSVOracle(t *testing.T) {
	for _, test := range []struct {
		data   string
		factor int
		want   bool
	}{
		{"region,total\nEast,30\nWest,10\n", 1, true},
		{"\ufeffregion,total\r\nWest,20.0\r\nEast,60\r\n", 2, true},
		{"region,total\nEast,30\nWest,10\n", 2, false},
		{"region,total\nEast,30\nEast,30\n", 1, false},
		{"region,total\nEast,NaN\nWest,10\n", 1, false},
		{"region,total\nEast,30\n", 1, false},
		{"region,total,extra\nEast,30,0\nWest,10,0\n", 1, false},
		{"region,total\nEast,30\nWest,10\nNorth,0\n", 1, false},
		{"Download /workspace/summary.csv", 1, false},
	} {
		if got := p5SalesCSVValid([]byte(test.data), test.factor); got != test.want {
			t.Fatalf("CSV %q factor %d: got %t", test.data, test.factor, got)
		}
	}
}
