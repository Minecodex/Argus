package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/config"
	api "github.com/kakj-go/Argus/internal/gen/openapi/presentationapi"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresPresentationReadOwnershipRevocationPagingAndHistoricalHash(t *testing.T) {
	address := os.Getenv("ARGUS_TEMPLATE_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned disposable template PostgreSQL")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, user, other, role, host, model, conversation, run, step, call, pid := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Template read fixture',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Readers')", d, e)
	for _, id := range []uuid.UUID{user, other} {
		exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Template reader')", id, e, d, id.String())
	}
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Template readers')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'conversation.use'),($1,'host.read')", role)
	for _, id := range []uuid.UUID{user, other} {
		exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, id, role)
	}
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Bound host','192.0.2.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", host, e)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), e, user, host)
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million) VALUES($1,$2,'Fixture','https://fixture.invalid','fixture','chat_completions',8192,1024,0,0)", model, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Read fixture',$4)", conversation, e, user, model)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'succeeded')", run, conversation, e, user, model)
	exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'tool_call','succeeded')", step, run, e)
	addCall := func(id uuid.UUID) {
		exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,input,input_hash,status) VALUES($1,$2,$3,$4,$5,'host.list','{}',decode(repeat('00',32),'hex'),'succeeded')", id, id.String(), e, run, step)
	}
	addCall(call)
	scope, err := presentation.Scope(ctx, store, e, user)
	if err != nil {
		t.Fatal(err)
	}
	asset := presentation.AssetV2("<section>Frozen template v2</section>", "fixture/v2", "result.detail", "result.page", "resources.context")
	if err = store.Queries.CreateTemplateAsset(ctx, db.CreateTemplateAssetParams{Hash: asset.Hash, Source: asset.Source, Runtime: asset.Runtime}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"id": host, "name": "<script>data</script>"}, map[string]any{"id": "another-sealed-row"}}, "Password": "never expose"})
	refs, _ := json.Marshal([]map[string]any{{"type": "host", "id": host}})
	_, err = store.Queries.CreateToolPresentation(ctx, db.CreateToolPresentationParams{ID: pid, EnterpriseID: e, ConversationID: conversation, ToolCallID: call, ToolVersion: asset.Version, AuthorizationScope: scope, TemplateHash: asset.Hash, DetailData: []byte(`{}`), ResourceRefs: refs, Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	binding := map[string]any{"kind": "sealed_result", "data": json.RawMessage(raw), "required": []string{"host.read"}}
	encoded, _ := json.Marshal(binding)
	if err = store.Queries.SetPresentationReadBinding(ctx, db.SetPresentationReadBindingParams{ID: pid, EnterpriseID: e, ReadOperations: asset.ReadOperations, ResultBinding: encoded}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	stamp := func(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
	tokens := map[uuid.UUID]string{}
	for _, id := range []uuid.UUID{user, other} {
		token := uuid.NewString()
		tokens[id] = token
		_, err = store.Queries.CreateSession(ctx, db.CreateSessionParams{ID: uuid.New(), TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash("csrf"), Audience: "enterprise", UserID: id, EnterpriseID: uuid.NullUUID{UUID: e, Valid: true}, DepartmentID: uuid.NullUUID{UUID: d, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	handler := PresentationHandler{Store: store, CursorSigner: pagination.Signer{Key: bytes.Repeat([]byte{8}, 32)}, Identity: EnterpriseIdentityHandler{Auth: SetupHandler{Config: config.Server{AllowedOrigins: []string{"http://argus.test"}}, Identity: identity.Service{Store: store}}}}
	requestCtx := func(id uuid.UUID) context.Context {
		request := httptest.NewRequest(http.MethodPost, "http://argus.test/api/v1/conversations/read", nil)
		request.Header.Set("Origin", "http://argus.test")
		request.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: tokens[id]})
		request.AddCookie(&http.Cookie{Name: csrfCookieName("enterprise"), Value: "csrf"})
		return WithRequestContext(ctx, httptest.NewRecorder(), request)
	}
	read := func(id, callID uuid.UUID, operation string, parameters map[string]any) api.ReadToolPresentationResponseObject {
		t.Helper()
		body := api.PresentationReadInput{Operation: api.PresentationReadInputOperation(operation)}
		parameterJSON, _ := json.Marshal(parameters)
		if err := json.Unmarshal(parameterJSON, &body.Parameters); err != nil {
			t.Fatal(err)
		}
		response, err := handler.ReadToolPresentation(requestCtx(id), api.ReadToolPresentationRequestObject{ConversationId: conversation, ToolCallId: callID, Body: &body, Params: api.ReadToolPresentationParams{XCSRFToken: "csrf"}})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	deny := func(response api.ReadToolPresentationResponseObject) {
		t.Helper()
		if _, ok := response.(api.ReadToolPresentation200JSONResponse); ok {
			t.Fatal("unauthorized read admitted")
		}
	}
	response := read(user, call, "result.detail", map[string]any{})
	filtered := read(user, call, "result.page", map[string]any{"search": "another-sealed-row"})
	if value, ok := filtered.(api.ReadToolPresentation200JSONResponse); !ok || value.Data["total"] != float64(1) || value.Data["source_total"] != float64(2) {
		t.Fatalf("sealed result search failed: %#v", filtered)
	}
	deny(read(other, call, "result.page", map[string]any{"search": "another-sealed-row"}))
	deny(read(user, call, "result.page", map[string]any{"filters": map[string]any{"resource_id": "other"}}))
	ready, ok := response.(api.ReadToolPresentation200JSONResponse)
	if !ok {
		t.Fatalf("authorized read: %#v", response)
	}
	if ready.Data["Password"] != nil {
		t.Fatal("credential leaked")
	}
	if ready.Data["items"].([]any)[0].(map[string]any)["name"] != "<script>data</script>" {
		t.Fatal("public data was interpreted")
	}
	deny(read(other, call, "result.detail", map[string]any{}))
	deny(read(user, uuid.New(), "result.detail", map[string]any{}))
	deny(read(user, call, "runtime.status", map[string]any{}))
	deny(read(user, call, "result.detail", map[string]any{"url": "https://outside.invalid"}))
	page := read(user, call, "result.page", map[string]any{"limit": float64(1)}).(api.ReadToolPresentation200JSONResponse)
	if page.NextCursor == nil {
		t.Fatal("missing bound cursor")
	}
	deny(read(user, call, "result.page", map[string]any{"cursor": *page.NextCursor + "tamper"}))
	deny(read(user, call, "result.page", map[string]any{"cursor": *page.NextCursor, "collection": "panels"}))
	oldCall, oldID := uuid.New(), uuid.New()
	addCall(oldCall)
	old := presentation.Asset("<section>Historical v1</section>", "fixture/v1")
	if err = store.Queries.CreateTemplateAsset(ctx, db.CreateTemplateAssetParams{Hash: old.Hash, Source: old.Source, Runtime: old.Runtime}); err != nil {
		t.Fatal(err)
	}
	_, err = store.Queries.CreateToolPresentation(ctx, db.CreateToolPresentationParams{ID: oldID, EnterpriseID: e, ConversationID: conversation, ToolCallID: oldCall, ToolVersion: old.Version, AuthorizationScope: scope, TemplateHash: old.Hash, DetailData: []byte(`{"name":"history"}`), ResourceRefs: []byte(`[]`), Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	historical, err := handler.GetToolPresentation(requestCtx(user), api.GetToolPresentationRequestObject{ConversationId: conversation, ToolCallId: oldCall})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := historical.(api.GetToolPresentation200JSONResponse); !ok || value.TemplateHash != old.Hash || value.TemplateSource != old.Source || string(value.Runtime) != "argus-template/v1" {
		t.Fatal("historical asset changed", historical)
	}
	deny(read(user, oldCall, "result.detail", map[string]any{}))
	exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_id=$2", e, user)
	deny(read(user, call, "resources.context", map[string]any{}))
	exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='host.read'", role)
	deny(read(user, call, "result.detail", map[string]any{}))
}
