package enterprisemcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/integration/remotemcp"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type fixtureResolver struct{}

func (fixtureResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestEnterpriseMCPPersistenceGrantsAndCredentialOwnership(t *testing.T) {
	url := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set ARGUS_P5_TEST_DATABASE_URL to a disposable migrated database")
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enterprise, department, admin, member, role, model, conversation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'P5',$2,'UTC')", enterprise, "p5-"+enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", department, enterprise)
	for _, user := range []uuid.UUID{admin, member} {
		exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'P5 user')", user, enterprise, department, "p5-"+user.String())
	}
	exec("INSERT INTO roles(id,enterprise_id,identity_key,name,builtin) VALUES($1,$2,'enterprise_admin','Administrator',true)", role, enterprise)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, admin, role)
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'P5','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, enterprise)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'P5',$4)", conversation, enterprise, member, model)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "keyring.json")
	key := []byte(strings.Repeat("k", 32))
	encoded, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(key)}})
	if err := os.WriteFile(keyPath, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	value := "p5-synthetic-mcp-credential"
	var writes atomic.Int32
	var changed atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(204)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+value {
			t.Error("MCP credential was not used by adapter")
		}
		var req struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": remotemcp.ProtocolVersion}
		case "notifications/initialized":
			w.WriteHeader(202)
			return
		case "tools/list":
			name := "write"
			if changed.Load() {
				name = "write_v2"
			}
			result = map[string]any{"tools": []any{map[string]any{"name": name, "inputSchema": map[string]any{"type": "object", "additionalProperties": false}}}}
		case "tools/call":
			writes.Add(1)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "done"}}}
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	base := server.Client().Transport
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.URL.Host = strings.TrimPrefix(server.URL, "https://")
		return base.RoundTrip(clone)
	})}
	credentials := secret.Service{Store: store, Keyring: keyring, Idempotency: postgres.Idempotency{Key: key}}
	service := Service{Store: store, Credentials: credentials, Idempotency: postgres.Idempotency{Key: key}, Policy: remotemcp.EndpointPolicy{Resolver: fixtureResolver{}}, HTTP: client}
	input := Input{Name: "Connection", Endpoint: "https://customer.example.test/mcp", AuthType: "bearer", Value: &value, Members: []uuid.UUID{member}}
	if _, err := service.Save(ctx, enterprise, member, uuid.Nil, input, uuid.NewString()); err == nil {
		t.Fatal("member configured a connection")
	}
	connection, err := service.Save(ctx, enterprise, admin, uuid.Nil, input, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	listed, err := service.List(ctx, enterprise, member, false)
	if err != nil || len(listed) != 1 {
		t.Fatalf("granted listing: %#v %v", listed, err)
	}
	public, _ := json.Marshal(listed)
	if strings.Contains(string(public), value) {
		t.Fatal("credential leaked")
	}
	revision, err := store.Queries.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: connection.ID, EnterpriseID: enterprise, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := store.Queries.GetCredential(ctx, db.GetCredentialParams{ID: revision.CredentialID.UUID, EnterpriseID: enterprise})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credentials.Get(ctx, enterprise, credential.SecretID); !errors.Is(err, secret.ErrMCPManagedSecret) {
		t.Fatalf("managed secret reachable through generic API: %v", err)
	}
	if _, err := credentials.CreateCredential(ctx, member.String(), enterprise, secret.CredentialInput{Name: "bypass", Protocol: "http", SecretID: credential.SecretID}, uuid.NewString()); !errors.Is(err, secret.ErrMCPManagedSecret) {
		t.Fatal("managed secret rebound")
	}
	if err := store.Queries.SelectConversationMCPConnection(ctx, db.SelectConversationMCPConnectionParams{ConversationID: conversation, EnterpriseID: enterprise, ConnectionID: connection.ID}); err != nil {
		t.Fatal(err)
	}
	p := toolruntime.Principal{EnterpriseID: enterprise, UserID: member, ConversationID: conversation, AuthorizationVersion: 1}
	part, err := service.BuildTools(ctx, p)
	if err != nil || len(part.Tools) != 1 {
		t.Fatalf("direct tool set: %#v %v", part, err)
	}
	result, err := part.Tools[0].Invoke(ctx, toolruntime.Invocation{Principal: p, ID: uuid.New(), Arguments: map[string]any{}})
	if err != nil || result.Data["text"] != "done" || writes.Load() != 1 {
		t.Fatalf("direct write: %#v %v", result, err)
	}
	if _, err := service.SetMembers(ctx, enterprise, admin, connection.ID, connection.Version, []uuid.UUID{}); err != nil {
		t.Fatal(err)
	}
	if _, err := part.Tools[0].Invoke(ctx, toolruntime.Invocation{Principal: p, ID: uuid.New(), Arguments: map[string]any{}}); err == nil || writes.Load() != 1 {
		t.Fatal("revoked cached tool still executed")
	}
	latest, err := service.Get(ctx, enterprise, admin, connection.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetMembers(ctx, enterprise, admin, connection.ID, latest.Version, []uuid.UUID{member}); err != nil {
		t.Fatal(err)
	}
	changed.Store(true)
	// The next Run must discover additions before any business invocation.
	fresh, err := service.BuildTools(ctx, p)
	if err != nil || len(fresh.Tools) != 1 {
		t.Fatalf("schema refresh unavailable: %v", err)
	}
	if fresh.Tools[0].Model.Name == part.Tools[0].Model.Name {
		t.Fatal("changed remote identity did not change alias")
	}
	if _, err := part.Tools[0].Invoke(ctx, toolruntime.Invocation{Principal: p, ID: uuid.New(), Arguments: map[string]any{}}); err == nil || writes.Load() != 1 {
		t.Fatal("changed remote schema executed cached arguments")
	}
	exec("UPDATE enterprise_users SET status='disabled' WHERE id=$1 AND enterprise_id=$2", member, enterprise)
	if _, err := fresh.Tools[0].Invoke(ctx, toolruntime.Invocation{Principal: p, ID: uuid.New(), Arguments: map[string]any{}}); err == nil || writes.Load() != 1 {
		t.Fatal("disabled member retained business invocation access through a cached grant")
	}
}
