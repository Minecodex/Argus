package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/platform"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	redisstore "github.com/kakj-go/Argus/internal/storage/redis"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestSetupHTTPBootstrapsBothPortals(t *testing.T) {
	redisURL := os.Getenv("ARGUS_SETUP_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("ARGUS_SETUP_TEST_REDIS_URL is not configured")
	}
	ctx := t.Context()
	store := postgrestest.New(t, "ARGUS_P5_TEST_DATABASE_URL", "setup_http", filepath.Join("..", "..", "..", "migrations", "postgresql"))
	cache, err := redisstore.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	const platformOrigin = "https://platform.example.test"
	const enterpriseOrigin = "https://argus.example.test"
	const username = "setup-admin"
	const password = "StrongSetup2026!"
	origins := []string{platformOrigin, enterpriseOrigin}
	token := platform.SetupTokenProvider{
		TokenPath: filepath.Join(t.TempDir(), "token"), ExpiresPath: filepath.Join(t.TempDir(), "expires"),
	}
	value := strings.Repeat("t", 40)
	if err := os.WriteFile(token.TokenPath, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token.ExpiresPath, []byte(time.Now().Add(time.Hour).Format(time.RFC3339)), 0o600); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{7}, 32)
	idempotency := postgres.Idempotency{Key: key}
	setup := SetupHandler{
		Config:   config.Server{AllowedOrigins: origins, RemoteOrigin: enterpriseOrigin},
		Setup:    platform.SetupService{Store: store, Idempotency: idempotency},
		Identity: identity.Service{Store: store, Redis: cache},
		Token:    token,
	}
	server := httptest.NewServer(NewRouterWithOptions(RouterOptions{
		Setup: &setup, AllowedOrigins: origins,
		Platform: &PlatformHandler{
			Auth: setup, Cursor: pagination.Signer{Key: key},
			Enterprise: platform.EnterpriseService{Store: store, Idempotency: idempotency},
		},
	}))
	t.Cleanup(server.Close)
	newClient := func() *http.Client {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Client{Jar: jar, Timeout: 10 * time.Second}
	}
	call := func(client *http.Client, method, path, origin string, expected int, payload any, headers map[string]string) map[string]any {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(ctx, method, server.URL+"/api/v1"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != expected {
			t.Fatalf("%s %s returned %d, want %d", method, path, response.StatusCode, expected)
		}
		if expected == http.StatusNoContent {
			return nil
		}
		var result map[string]any
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	platformClient, enterpriseClient := newClient(), newClient()
	init := call(platformClient, http.MethodPost, "/setup/initialize", platformOrigin, http.StatusCreated, map[string]any{
		"enterprise_name": "首家企业", "timezone": "Asia/Shanghai", "username": username, "display_name": "管理员", "password": password,
	}, map[string]string{"X-Argus-Setup-Token": value, "Idempotency-Key": uuid.NewString()})
	credentials := map[string]any{"username": username, "password": password}
	enterpriseLogin := call(enterpriseClient, http.MethodPost, "/enterprise/auth/login", enterpriseOrigin, http.StatusOK, credentials, nil)
	enterpriseSession := enterpriseLogin["authenticated_session"].(map[string]any)
	if enterpriseSession["user"].(map[string]any)["id"] != init["enterprise_user_id"] {
		t.Fatal("enterprise login did not resolve to the initialized enterprise account")
	}
	call(enterpriseClient, http.MethodPut, "/enterprise/account/password", enterpriseOrigin, http.StatusNoContent, map[string]any{
		"current_password": password, "new_password": "NewEnterprise2027!", "expected_version": 1,
	}, map[string]string{"X-CSRF-Token": enterpriseSession["csrf_token"].(string)})
	call(enterpriseClient, http.MethodPost, "/enterprise/auth/login", enterpriseOrigin, http.StatusUnauthorized, credentials, nil)
	platformLogin := call(platformClient, http.MethodPost, "/platform/auth/login", platformOrigin, http.StatusOK, credentials, nil)
	platformSession := platformLogin["authenticated_session"].(map[string]any)
	if platformSession["user"].(map[string]any)["id"] != init["platform_user_id"] {
		t.Fatal("platform login did not resolve to the initialized platform account")
	}
	enterprise := call(platformClient, http.MethodPost, "/platform/enterprises", platformOrigin, http.StatusCreated,
		map[string]any{"name": "首家企业", "timezone": "UTC"},
		map[string]string{"X-CSRF-Token": platformSession["csrf_token"].(string), "Idempotency-Key": uuid.NewString()})
	id, err := uuid.Parse(enterprise["id"].(string))
	if _, hasCode := enterprise["code"]; err != nil || id.String() == init["enterprise_id"] || hasCode {
		t.Fatal("enterprise creation did not assign a distinct UUID")
	}
}
