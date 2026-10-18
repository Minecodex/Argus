package argusdev

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func existingPortalBrowserCookies(env *E2EEnvironment) (string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	endpoint, err := url.Parse(env.EnterpriseOrigin() + "/api/v1/enterprise/auth/session")
	if err != nil || endpoint.Scheme != "https" {
		return "", fmt.Errorf("existing portal requires HTTPS")
	}
	values := []map[string]any{}
	for _, cookie := range client.Client("enterprise").Jar.Cookies(endpoint) {
		values = append(values, map[string]any{"name": cookie.Name, "value": cookie.Value, "domain": endpoint.Hostname(), "path": "/", "httpOnly": !strings.Contains(cookie.Name, "csrf"), "secure": true, "sameSite": "Strict"})
	}
	if len(values) == 0 {
		return "", fmt.Errorf("existing portal has no authenticated session cookie")
	}
	raw, err := json.Marshal(values)
	return string(raw), err
}
