package conversation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type capacityTools struct{ extra int }

func (p *capacityTools) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	part := toolruntime.Contribution{NativeCatalog: "capacity-test"}
	for _, name := range []string{"tool.search", "tool.describe", "tool.invoke", "mcp_capacity_test"} {
		source, description := "argus", "test"
		if strings.HasPrefix(name, "mcp_") {
			source, description = "external_mcp", strings.Repeat("x", p.extra)
		}
		part.Tools = append(part.Tools, toolruntime.Tool{Definition: toolruntime.Definition{
			Model: modelprovider.Tool{Name: name, Description: description, Schema: map[string]any{"type": "object"}}, Source: source, Version: "1"},
			Invoke: func(context.Context, toolruntime.Invocation) (toolruntime.Result, error) {
				panic("preflight executed a business tool")
			}})
	}
	return part, nil
}

func TestMessageAdmissionRejectsFixedInputAtRuntimeBoundary(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, u, m := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Capacity',$2,'UTC')", e, "capacity-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Capacity')", u, e, d, "capacity-"+u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Capacity','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", m, e)
	defer func() {
		_, _ = store.Pool.Exec(context.Background(), "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", e)
	}()
	tools := &capacityTools{}
	svc := Service{Store: store, Tools: toolruntime.CompositeFactory{Providers: []toolruntime.Provider{tools}}, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}}
	for _, protocol := range []string{"chat_completions", "responses"} {
		exec("UPDATE ai_models SET api_protocol=$2 WHERE id=$1", m, protocol)
		for _, delta := range []int{-1, 0, 1, 1985} {
			c := uuid.New()
			exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Capacity',$4)", c, e, u, m)
			tools.extra = 1
			base, err := svc.Preflight(t.Context(), e, u, c, "中文分析", nil)
			if err != nil {
				t.Fatal(err)
			}
			hard := modelprovider.NewContextBudget(32768, 1024).HardLimit
			tools.extra = 1 + hard + delta - base.EstimatedTokens
			value, err := svc.Preflight(t.Context(), e, u, c, "中文分析", nil)
			if err != nil {
				t.Fatal(err)
			}
			if value.EstimatedTokens != hard+delta || value.Ready != (delta < 0) {
				t.Fatalf("%s delta=%d estimated=%d ready=%v", protocol, delta, value.EstimatedTokens, value.Ready)
			}
			_, err = svc.AddMessage(t.Context(), u.String(), e, u, c, 1, "zh-CN", "中文分析", nil, uuid.NewString())
			if delta < 0 {
				if err != nil {
					t.Fatal(err)
				}
				continue
			}
			var coded toolruntime.Error
			if !errors.As(err, &coded) || coded.Kind != "MODEL_TOOL_CAPACITY_EXCEEDED" {
				t.Fatalf("wrong admission result: %v", err)
			}
			var runs, tasks, events int
			if err := store.Pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM runs WHERE conversation_id=$1),(SELECT count(*) FROM runtime_tasks WHERE run_id IN(SELECT id FROM runs WHERE conversation_id=$1)),(SELECT count(*) FROM conversation_events WHERE conversation_id=$1)", c).Scan(&runs, &tasks, &events); err != nil {
				t.Fatal(err)
			}
			if runs != 0 || tasks != 0 || events != 0 {
				t.Fatalf("rejected message changed state: %d/%d/%d", runs, tasks, events)
			}
		}
	}
}
