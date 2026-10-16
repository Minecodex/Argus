// Creates an owned, empty tenant for the formal Chatbox loading check. It
// executes the real host.list tool, never a model or fabricated telemetry.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolgateway"
)

type fixture struct {
	Enterprise, User, Department, Role, Model, Conversation uuid.UUID
	Code, Username, Password                                string
}

func main() {
	if len(os.Args) != 3 || (os.Args[1] != "create" && os.Args[1] != "clean") {
		panic("usage: template-release-smoke create|clean private-manifest.json")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	store, err := postgres.Open(ctx, os.Getenv("ARGUS_TEMPLATE_RELEASE_DATABASE_URL"))
	must(err)
	defer store.Close()
	if os.Args[1] == "clean" {
		var f fixture
		raw, err := os.ReadFile(os.Args[2])
		must(err)
		must(json.Unmarshal(raw, &f))
		must(clean(ctx, store, f))
		fmt.Println("Owned formal Chatbox smoke tenant removed")
		return
	}
	f := fixture{Enterprise: uuid.New(), User: uuid.New(), Department: uuid.New(), Role: uuid.New(), Model: uuid.New(), Conversation: uuid.New(), Password: "Aa9!" + uuid.NewString()}
	f.Code, f.Username = "template-smoke-"+f.Enterprise.String(), "template_smoke_"+strings.ReplaceAll(f.User.String(), "-", "")
	// Write ownership before any mutation so a failed setup can be cleaned.
	raw, _ := json.Marshal(f)
	must(os.WriteFile(os.Args[2], raw, 0600))
	must(create(ctx, store, f))
	fmt.Println("Owned empty tenant and real v2 host.list presentation created; no model or telemetry fixture")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func withTx(ctx context.Context, store *postgres.Store, action func(pgx.Tx, *db.Queries) error) error {
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = action(tx, db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func create(ctx context.Context, store *postgres.Store, f fixture) error {
	hash, err := identity.HashPassword(f.Password)
	if err != nil {
		return err
	}
	err = withTx(ctx, store, func(tx pgx.Tx, q *db.Queries) error {
		queries := []struct {
			sql  string
			args []any
		}{
			{"INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Template release smoke (owned empty tenant)',$2,'Asia/Shanghai')", []any{f.Enterprise, f.Code}},
			{"INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Smoke',true)", []any{f.Department, f.Enterprise}},
			{"INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'模板发布验收')", []any{f.User, f.Enterprise, f.Department, f.Username}},
			{"INSERT INTO password_credentials(id,audience,subject_id,encoded_hash,temporary) VALUES($1,'enterprise',$2,$3,false)", []any{uuid.New(), f.User, hash}},
			{"INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Smoke template viewer')", []any{f.Role, f.Enterprise}},
			{"INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'conversation.use'),($1,'conversation.read'),($1,'host.read'),($1,'workspace.use')", []any{f.Role}},
			{"INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", []any{uuid.New(), f.Enterprise, f.User, f.Role}},
			{"INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million) VALUES($1,$2,'Unused smoke model','https://unused.invalid','unused','chat_completions',8192,1024,0,0)", []any{f.Model, f.Enterprise}},
			{"INSERT INTO model_quotas(id,enterprise_id,model_id,subject_type,subject_id,monthly_amount) VALUES($1,$2,$3,'user',$4,1)", []any{uuid.New(), f.Enterprise, f.Model, f.User}},
			{"INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'正式发布模板加载复核（独立空企业）',$4)", []any{f.Conversation, f.Enterprise, f.User, f.Model}},
		}
		for _, statement := range queries {
			if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
				return err
			}
		}
		return audit.InitializeChain(ctx, q, "enterprise", uuid.NullUUID{UUID: f.Enterprise, Valid: true})
	})
	if err != nil {
		return err
	}
	registry := mcp.NewRegistry()
	base := toolgateway.ResourceTools{Store: store, Resources: resource.Service{Store: store}}
	if err := base.Register(registry); err != nil {
		return err
	}
	run, step, call, pid := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	native := mcp.Call{ToolID: "host.list", Caller: "model", CallID: call.String(), Enterprise: f.Enterprise.String(), Subject: f.User.String(), SubjectType: "user", RunID: run.String(), InvocationID: call.String(), Input: map[string]any{}}
	result, err := registry.Call(ctx, native)
	if err != nil {
		return err
	}
	metadata, _ := registry.Lookup("host.list")
	p, err := metadata.Present(ctx, native, result)
	if err != nil {
		return err
	}
	details, _ := json.Marshal(p.Data)
	refs, _ := json.Marshal(p.Resources)
	p.Binding["required"], p.Binding["tool_id"] = metadata.Required, metadata.ID
	binding, _ := json.Marshal(p.Binding)
	return withTx(ctx, store, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := tx.Exec(ctx, "INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'succeeded')", run, f.Conversation, f.Enterprise, f.User, f.Model); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'tool_call','succeeded')", step, run, f.Enterprise); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,input,input_hash,status) VALUES($1,$2,$3,$4,$5,'host.list','{}',decode(repeat('00',32),'hex'),'succeeded')", call, call.String(), f.Enterprise, run, step); err != nil {
			return err
		}
		if err := q.CreateTemplateAsset(ctx, db.CreateTemplateAssetParams{Hash: p.Hash, Source: p.Template, Runtime: p.Runtime}); err != nil {
			return err
		}
		if _, err := q.CreateToolPresentation(ctx, db.CreateToolPresentationParams{ID: pid, EnterpriseID: f.Enterprise, ConversationID: f.Conversation, ToolCallID: call, ToolVersion: p.Version, AuthorizationScope: p.AuthorizationScope, TemplateHash: p.Hash, DetailData: details, ResourceRefs: refs, Status: "ready"}); err != nil {
			return err
		}
		if err := q.SetPresentationReadBinding(ctx, db.SetPresentationReadBindingParams{ID: pid, EnterpriseID: f.Enterprise, ReadOperations: p.ReadOperations, ResultBinding: binding}); err != nil {
			return err
		}
		// Record the actual native invocation so the public parameter/result tabs
		// exercise ledger replay as well as the independently stored presentation.
		for _, event := range []struct {
			kind    string
			payload map[string]any
		}{
			{"assistant_message", map[string]any{"tool_calls": []map[string]any{{"id": call.String(), "name": native.ToolID, "arguments": native.Input}}}},
			{"tool_call_result", map[string]any{"tool_call_id": call.String(), "tool_id": native.ToolID, "source": "argus", "status": "succeeded", "projection": map[string]any{"summary": result.Structured}}},
		} {
			if _, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: f.Enterprise, ConversationID: f.Conversation, RunID: uuid.NullUUID{UUID: run, Valid: true}, Type: event.kind, ActorType: "service", Payload: event.payload, Classification: "internal"}); err != nil {
				return err
			}
		}
		_, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: f.Enterprise, ConversationID: f.Conversation, RunID: uuid.NullUUID{UUID: run, Valid: true}, Type: "tool_presentation", ActorType: "service", Payload: map[string]any{"tool_call_id": call.String(), "presentation_id": pid.String(), "status": "ready"}, Classification: "internal"})
		return err
	})
}

func clean(ctx context.Context, store *postgres.Store, f fixture) error {
	var code string
	err := store.Pool.QueryRow(ctx, "SELECT code FROM enterprises WHERE id=$1", f.Enterprise).Scan(&code)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if code != f.Code || !strings.HasPrefix(code, "template-smoke-") || f.User == uuid.Nil {
		return fmt.Errorf("smoke ownership changed")
	}
	return withTx(ctx, store, func(tx pgx.Tx, q *db.Queries) error {
		// A development acceptance tenant has no external relationships. Disable
		// trigger checks in this transaction only; every delete is UUID-scoped.
		if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role=replica"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM role_permissions WHERE role_id=$1", f.Role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM password_credentials WHERE audience='enterprise' AND subject_id=$1", f.User); err != nil {
			return err
		}
		rows, err := store.Pool.Query(ctx, "SELECT table_name FROM information_schema.columns WHERE table_schema='public' AND column_name='enterprise_id' ORDER BY table_name")
		if err != nil {
			return err
		}
		tables := []string{}
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				rows.Close()
				return err
			}
			tables = append(tables, table)
		}
		rows.Close()
		for _, table := range tables {
			if _, err := tx.Exec(ctx, "DELETE FROM "+pgx.Identifier{"public", table}.Sanitize()+" WHERE enterprise_id=$1", f.Enterprise); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "DELETE FROM enterprises WHERE id=$1 AND code=$2", f.Enterprise, f.Code)
		return err
	})
}
