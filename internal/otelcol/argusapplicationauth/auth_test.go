package argusapplicationauth

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/collector/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testAuthority struct {
	calls       int
	revoked     bool
	unavailable bool
}

func (a *testAuthority) AuthorizeApplication(_ context.Context, protocol, token string) (map[string]string, error) {
	a.calls++
	if a.unavailable {
		return nil, status.Error(codes.Unavailable, "private infrastructure detail")
	}
	if a.revoked || protocol != "otlp" || token != "argus_app.test-token" {
		return nil, errors.New("rejected")
	}
	return map[string]string{"key_id": "key", "endpoint_id": "endpoint", "endpoint_version": "2", "signals": "traces,logs"}, nil
}
func TestApplicationAuthenticationRejectsAmbiguousHeadersAndRechecksKeys(t *testing.T) {
	authority := &testAuthority{}
	auth := authentication{config: Config{Protocol: "otlp"}, authority: authority}
	ctx, err := auth.Authenticate(t.Context(), map[string][]string{"Authorization": {"Bearer argus_app.test-token"}})
	if err != nil || client.FromContext(ctx).Auth.GetAttribute("endpoint_id") != "endpoint" {
		t.Fatalf("identity context: %v", err)
	}
	for _, headers := range []map[string][]string{nil, {"Authorization": {"argus_app.test-token"}}, {"Authorization": {"Bearer argus_app.test-token", "Bearer argus_app.other"}}, {"Authorization": {"Bearer argus_app.test-token"}, "authorization": {"Bearer argus_app.test-token"}}} {
		if _, err = auth.Authenticate(t.Context(), headers); err == nil {
			t.Fatal("ambiguous or missing credential accepted")
		}
	}
	authority.revoked = true
	if _, err = auth.Authenticate(t.Context(), map[string][]string{"authorization": {"Bearer argus_app.test-token"}}); err == nil || authority.calls != 2 {
		t.Fatal("revocation bypassed by cached authentication")
	}
}

func TestUnavailableAuthorityCarriesNoAuthorizedIdentity(t *testing.T) {
	auth := authentication{config: Config{Protocol: "otlp"}, authority: &testAuthority{unavailable: true}}
	ctx, err := auth.Authenticate(t.Context(), map[string][]string{"authorization": {"Bearer argus_app.test-token"}})
	if err != nil {
		t.Fatal(err)
	}
	info := client.FromContext(ctx)
	if info.Auth.GetAttribute("transient_error") == "" || info.Auth.GetAttribute("key_id") != "" || info.Auth.GetAttribute("endpoint_id") != "" {
		t.Fatal("transient failure manufactured authorization")
	}
}
