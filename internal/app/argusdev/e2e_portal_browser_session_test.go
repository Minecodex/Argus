package argusdev

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestOwnedPortalCookieHandoffUsesActualAPIOriginAndEnterpriseAudience(t *testing.T) {
	api := NewScenarioHTTP("https://platform.owned.argus.test/api/v1", t.TempDir(), nil)
	env := &E2EEnvironment{Endpoints: &E2EEndpoints{EnterpriseOrigin: "https://enterprise.owned.argus.test"}, State: NewScenarioState("owned-cookie-observation")}
	env.State.HTTP = api
	origin, _ := url.Parse(api.BaseURL + "/enterprise/auth/session")
	api.Client("enterprise").Jar.SetCookies(origin, []*http.Cookie{{Name: "argus_enterprise_session", Value: "owned-session", Path: "/", Secure: true, HttpOnly: true}, {Name: "argus_enterprise_csrf", Value: "owned-csrf", Path: "/", Secure: true}, {Name: "argus_platform_session", Value: "must-not-transfer", Path: "/", Secure: true}})
	raw, err := existingPortalBrowserCookies(env)
	if err != nil {
		t.Fatal(err)
	}
	var cookies []struct {
		Name, Value, Domain, Path, SameSite string
		HTTPOnly, Secure                    bool
	}
	if err = json.Unmarshal([]byte(raw), &cookies); err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 2 {
		t.Fatal("enterprise session handoff exported another audience")
	}
	for _, cookie := range cookies {
		if cookie.Domain != "enterprise.owned.argus.test" || cookie.Path != "/" || !cookie.Secure || cookie.SameSite != "Strict" {
			t.Fatal("browser origin or cookie protection changed")
		}
		if (cookie.Name == "argus_enterprise_session") != cookie.HTTPOnly {
			t.Fatal("cookie visibility changed")
		}
	}
	api.Reset("enterprise")
	api.Client("enterprise").Jar.SetCookies(origin, []*http.Cookie{{Name: "argus_enterprise_csrf", Value: "csrf-alone", Path: "/", Secure: true}})
	if _, err = existingPortalBrowserCookies(env); err == nil {
		t.Fatal("CSRF cookie alone was treated as authentication")
	}
}
