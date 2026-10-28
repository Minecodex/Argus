package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/agent"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// The Windows host's DNS returns 198.18/15 fake IPs. Resolve this explicitly
// authorized public provider through DNS-over-HTTPS for acceptance, then keep
// the production endpoint policy, IP checks, pinned dialing and TLS verification.
type modelGovernancePublicResolver struct{}

func (modelGovernancePublicResolver) LookupNetIP(ctx context.Context, _, host string) ([]netip.Addr, error) {
	if host != "api.apimart.ai" {
		return nil, errors.New("unapproved live acceptance host")
	}
	if pinned := os.Getenv("ARGUS_MODEL_GOVERNANCE_PUBLIC_IPS"); pinned != "" {
		addresses := []netip.Addr{}
		for _, text := range strings.Split(pinned, ",") {
			address, err := netip.ParseAddr(text)
			if err != nil {
				return nil, err
			}
			addresses = append(addresses, address)
		}
		return addresses, nil
	}
	request, _ := http.NewRequestWithContext(ctx, "GET", "https://dns.google/resolve?name="+url.QueryEscape(host)+"&type=A", nil)
	client := http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("public DNS lookup unavailable")
	}
	var answer struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&answer); err != nil {
		return nil, err
	}
	addresses := []netip.Addr{}
	for _, row := range answer.Answer {
		if row.Type == 1 {
			if address, e := netip.ParseAddr(row.Data); e == nil {
				addresses = append(addresses, address)
			}
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("public DNS has no addresses")
	}
	return addresses, nil
}

type modelGovernanceProvider struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

func modelGovernanceTools(t *testing.T) toolruntime.Factory {
	t.Helper()
	gateway, err := toolgateway.New(mcp.NewRegistry(), "model-governance-acceptance")
	if err != nil {
		t.Fatal(err)
	}
	return toolruntime.CompositeFactory{Providers: []toolruntime.Provider{gateway}}
}

type modelGovernanceCompatibilityRecorder struct {
	t      *testing.T
	tester model.ProviderTester
}
type modelGovernanceTransport struct {
	t    *testing.T
	next http.RoundTripper
}

func (r modelGovernanceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.next.RoundTrip(request)
	if err != nil {
		r.t.Logf("live provider transport failed: %T %v", err, err)
	}
	return response, err
}

func (r modelGovernanceCompatibilityRecorder) Test(ctx context.Context, input model.Input) model.CompatibilityResult {
	result := r.tester.Test(ctx, input)
	r.t.Logf("live compatibility: %+v", result.Checks)
	return result
}

func configureModelGovernanceLive(t *testing.T, handler *ModelHandler) modelprovider.PublicEndpointPolicy {
	t.Helper()
	policy := modelprovider.PublicEndpointPolicy{Resolver: modelGovernancePublicResolver{}}
	if os.Getenv("ARGUS_MODEL_GOVERNANCE_PROVIDER_FILE") == "" {
		return policy
	}
	wrappingKey := make([]byte, 32)
	if _, err := rand.Read(wrappingKey); err != nil {
		t.Fatal(err)
	}
	keyData, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(wrappingKey)}})
	clear(wrappingKey)
	keyPath := filepath.Join(t.TempDir(), "keyring.json")
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	handler.Service.Keyring = keyring
	client := policy.ModelClient()
	client.Transport = modelGovernanceTransport{t: t, next: client.Transport}
	handler.Service.Tester = modelGovernanceCompatibilityRecorder{t: t, tester: model.ProviderTester{Policy: policy, Client: client}}
	return policy
}

func runModelGovernanceLive(t *testing.T, f *auditWorkflowFixture, models ModelHandler, policy modelprovider.PublicEndpointPolicy, projector modelmetrics.Projector) (uuid.UUID, string) {
	t.Helper()
	file := os.Getenv("ARGUS_MODEL_GOVERNANCE_PROVIDER_FILE")
	if file == "" {
		return uuid.Nil, ""
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("live provider file unavailable")
	}
	var provider modelGovernanceProvider
	if json.Unmarshal(raw, &provider) != nil || provider.APIKey == "" {
		t.Fatal("invalid live provider configuration")
	}
	clear(raw)
	// Keys stay in this request and the isolated encrypted credential store only.
	body := map[string]any{"name": "APImart live governance", "base_url": provider.BaseURL, "model_id": provider.Model, "api_key": provider.APIKey, "api_protocol": "chat_completions", "context_window_tokens": 32768, "max_output_tokens": 4096, "reasoning_effort": "low", "input_price_per_million": 1, "output_price_per_million": 2}
	created := f.call("POST", "/enterprise/ai-models/test-and-create", body, 201, uuid.NewString())
	delete(body, "api_key")
	provider.APIKey = ""
	var result struct {
		Model struct {
			ID uuid.UUID `json:"id"`
		} `json:"model"`
	}
	if json.Unmarshal(created, &result) != nil || result.Model.ID == uuid.Nil {
		t.Fatal("live model not created")
	}
	var c struct {
		ID uuid.UUID `json:"id"`
	}
	if json.Unmarshal(f.call("POST", "/conversations", map[string]any{"title": "Model governance live acceptance", "selected_model_id": result.Model.ID}, 201, uuid.NewString()), &c) != nil || c.ID == uuid.Nil {
		t.Fatal("live conversation not created")
	}
	accepted := f.call("POST", "/conversations/"+c.ID.String()+"/messages", map[string]any{"content": "Reply with exactly OK. Do not use tools.", "file_ids": []any{}}, 202, uuid.NewString())
	var message struct {
		Run struct {
			ID uuid.UUID `json:"run_id"`
		} `json:"run"`
	}
	if json.Unmarshal(accepted, &message) != nil || message.Run.ID == uuid.Nil {
		t.Fatal("live run not accepted")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	loop := &agent.Loop{Store: f.store, Models: models.Service, Tools: modelGovernanceTools(t), EndpointPolicy: policy}
	processor := runtime.Processor{Store: f.store, Queue: "agent", Owner: "modelgov-live", Lease: time.Minute, Poll: 50 * time.Millisecond, Handle: loop, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	done := make(chan error, 1)
	go func() { done <- processor.Run(ctx) }()
	status := ""
	var failure *string
	for ctx.Err() == nil {
		if err = f.store.Pool.QueryRow(ctx, `SELECT status,error_code FROM runs WHERE id=$1`, message.Run.ID).Scan(&status, &failure); err != nil {
			t.Fatal(err)
		}
		if status == "succeeded" || status == "failed" || status == "cancelled" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	<-done
	if status != "succeeded" {
		if failure != nil {
			t.Fatalf("live run ended %s (%s)", status, *failure)
		}
		t.Fatalf("live run ended %s", status)
	}
	for {
		n, e := projector.Batch(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			break
		}
	}
	var count, input, output, succeeded, failed, reportedSuccesses, estimatedInput, estimatedOutput int64
	var amount float64
	var complete bool
	err = f.store.Pool.QueryRow(t.Context(), `SELECT count(*),COALESCE(sum(input_tokens) FILTER(WHERE input_usage_source='provider'),0)::bigint,COALESCE(sum(output_tokens) FILTER(WHERE output_usage_source='provider'),0)::bigint,sum(amount)::float8,bool_and(input_usage_source='provider' AND output_usage_source='provider'),count(*) FILTER(WHERE status='succeeded'),count(*) FILTER(WHERE status='failed'),count(*) FILTER(WHERE status='succeeded' AND input_usage_source='provider' AND output_usage_source='provider'),COALESCE(sum(input_tokens) FILTER(WHERE input_usage_source='estimated'),0)::bigint,COALESCE(sum(output_tokens) FILTER(WHERE output_usage_source='estimated'),0)::bigint FROM model_calls WHERE enterprise_id=$1 AND model_id=$2 AND completed_at IS NOT NULL`, f.enterprise, result.Model.ID).Scan(&count, &input, &output, &amount, &complete, &succeeded, &failed, &reportedSuccesses, &estimatedInput, &estimatedOutput)
	if err != nil || succeeded < 1 || reportedSuccesses != succeeded || input <= 0 || output <= 0 {
		t.Fatalf("successful live calls must have provider usage: calls=%d succeeded=%d reported_successes=%d input=%d output=%d error=%v", count, succeeded, reportedSuccesses, input, output, err)
	}
	month := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01")
	id := modelmetrics.DashboardID(f.enterprise)
	query := map[string]any{"resource_ids": []string{}, "variables": map[string]any{"month": map[string]any{"all": false, "values": []string{month}}, "model": map[string]any{"all": false, "values": []string{result.Model.ID.String()}}}, "local_values": map[string]any{}}
	var execution dashboard.Execution
	if err = json.Unmarshal(f.call("POST", "/dashboards/"+id.String()+"/execute", query, 200, ""), &execution); err != nil {
		t.Fatal(err)
	}
	assertModelMetric(t, execution, "requests", float64(count))
	assertModelMetric(t, execution, "tokens", float64(input+output))
	assertModelMetric(t, execution, "amount", amount)
	assertModelMetric(t, execution, "errors", float64(failed))
	assertModelMetric(t, execution, "success", float64(succeeded)/float64(count))
	if execution.ModelGovernance == nil || execution.ModelGovernance.UsageComplete != complete || execution.ModelGovernance.EstimatedInput != estimatedInput || execution.ModelGovernance.EstimatedOutput != estimatedOutput {
		t.Fatal("live incomplete usage was masked or estimates were mixed into reported totals")
	}
	t.Logf("LIVE APImart %s: HTTP -> Agent -> PostgreSQL outbox -> ClickHouse -> mTLS PromQL -> dashboard; calls=%d succeeded=%d failed=%d reported_input=%d reported_output=%d all_usage_complete=%v amount=%g at configured test rates 1/2", provider.Model, count, succeeded, failed, input, output, complete, amount)
	receipt, _ := json.MarshalIndent(map[string]any{"provider": "APImart", "model": provider.Model, "calls": count, "succeeded_calls": succeeded, "failed_calls": failed, "successful_calls_with_reported_usage": reportedSuccesses, "input_tokens": input, "output_tokens": output, "estimated_input_tokens": estimatedInput, "estimated_output_tokens": estimatedOutput, "amount": amount, "provider_usage_complete": complete, "test_input_price": 1, "test_output_price": 2, "pipeline": "HTTP -> Agent -> PostgreSQL outbox -> ClickHouse -> mTLS PromQL -> Dashboard"}, "", "  ")
	if path := os.Getenv("ARGUS_MODEL_GOVERNANCE_RECEIPT"); path != "" {
		if err = os.WriteFile(path, receipt, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return result.Model.ID, month
}
