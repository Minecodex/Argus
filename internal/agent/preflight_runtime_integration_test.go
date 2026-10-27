package agent

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type fixedContextTools struct{}

func (fixedContextTools) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	part := toolruntime.Contribution{NativeCatalog: "fixed-context-test"}
	for _, name := range []string{"tool.search", "tool.describe", "tool.invoke"} {
		part.Tools = append(part.Tools, toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: name, Description: "按授权类别查询，不执行敏感输入", Schema: map[string]any{"type": "object"}}, Source: "argus", Version: "1"}, Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
			panic("context measurement must not invoke tools")
		}})
	}
	return part, nil
}

func TestPreflightMeasuresSameFixedRequestAsAgentRuntime(t *testing.T) {
	for _, protocol := range []string{"chat_completions", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			f := newRecoveryFixture(t)
			ctx := t.Context()
			capabilities := []byte(`{"output_token_parameter":"max_tokens","reasoning_effort":"low"}`)
			if protocol == "responses" {
				capabilities = []byte(`{"output_token_parameter":"max_output_tokens"}`)
			}
			f.exec("UPDATE ai_models SET api_protocol=$2,capabilities=$3 WHERE id=$1", f.m, protocol, capabilities)
			factory := toolruntime.CompositeFactory{Providers: []toolruntime.Provider{fixedContextTools{}}}
			remote := func(_ context.Context, run db.Run) (map[string]any, error) {
				if run.EnterpriseID != f.e || run.ActorUserID != f.u || run.ConversationID != f.c {
					t.Fatal("fixed facts escaped the current principal")
				}
				return map[string]any{"requests": []any{}, "results_directory": "/workspace/results", "mode": run.ModeSnapshot}, nil
			}
			service := conversation.Service{Store: f.store, Tools: factory, RemoteFacts: remote, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}}
			principal, err := service.ToolPrincipal(ctx, f.e, f.u, f.c)
			if err != nil {
				t.Fatal(err)
			}
			set, err := factory.Build(ctx, principal)
			if err != nil {
				t.Fatal(err)
			}
			f.exec("UPDATE runs SET status='cancelled' WHERE id=$1", f.r)
			admission, err := service.Preflight(ctx, f.e, f.u, f.c, "中文分析", nil)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := service.AddMessage(ctx, f.u.String(), f.e, f.u, f.c, 1, "zh-CN", "中文分析", nil, uuid.NewString())
			if err != nil {
				t.Fatal(err)
			}
			f.r = accepted.Run.ID
			loop := Loop{Store: f.store, RemoteFacts: remote}
			run := f.run()
			messages, input, _, source, err := loop.messages(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			revision := db.AiModelRevision{ProviderModelID: "model", ApiProtocol: protocol, ContextWindowTokens: 32768, MaxOutputTokens: 1024, Capabilities: capabilities}
			projection, err := loop.context(ctx, run, revision, messages, input, set.Models(), source)
			if err != nil {
				t.Fatal(err)
			}
			parameter, err := modelprovider.BudgetParameterForCapabilities(modelprovider.Protocol(protocol), capabilities)
			if err != nil {
				t.Fatal(err)
			}
			effort, err := modelprovider.ReasoningEffortForCapabilities(modelprovider.Protocol(protocol), capabilities)
			if err != nil {
				t.Fatal(err)
			}
			bytes, err := (modelprovider.Provider{Protocol: modelprovider.Protocol(protocol), BaseURL: "https://model.example.test"}).RequestBytes(modelprovider.Request{Model: "model", Messages: projection.Messages, Tools: set.Models(), MaxTokens: 1024, OutputTokenParameter: parameter, ReasoningEffort: effort})
			if err != nil {
				t.Fatal(err)
			}
			if admission.EstimatedTokens != bytes {
				t.Fatalf("admission measured %d bytes but runtime sends %d", admission.EstimatedTokens, bytes)
			}
			if admission.Ready != modelprovider.NewContextBudget(32768, 1024).Fits(bytes) {
				t.Fatal("admission and runtime disagree at the hard boundary")
			}
			if calls := f.count("SELECT count(*) FROM model_calls WHERE run_id=$1", f.r); calls != 0 {
				t.Fatal("capacity measurement made a provider call")
			}
		})
	}
}
