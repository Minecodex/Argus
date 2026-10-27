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
	// API login may use the platform origin of the same owned deployment.
	// Export only enterprise cookies to its enterprise browser origin.
	apiEndpoint, err := url.Parse(client.BaseURL + "/enterprise/auth/session")
	if err != nil || apiEndpoint.Scheme != "https" || apiEndpoint.Hostname() == "" {
		return "", fmt.Errorf("existing portal requires an HTTPS API session origin")
	}
	values := []map[string]any{}
	authenticated := false
	for _, cookie := range client.Client("enterprise").Jar.Cookies(apiEndpoint) {
		if cookie.Name != "argus_enterprise_session" && cookie.Name != "argus_enterprise_csrf" {
			continue
		}
		authenticated = authenticated || cookie.Name == "argus_enterprise_session"
		values = append(values, map[string]any{"name": cookie.Name, "value": cookie.Value, "domain": endpoint.Hostname(), "path": "/", "httpOnly": !strings.Contains(cookie.Name, "csrf"), "secure": true, "sameSite": "Strict"})
	}
	if !authenticated {
		return "", fmt.Errorf("existing portal has no authenticated session cookie")
	}
	raw, err := json.Marshal(values)
	return string(raw), err
}
