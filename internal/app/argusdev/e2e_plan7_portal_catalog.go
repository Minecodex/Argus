package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// Existing installations contain historical and other-platform catalog rows.
// Pin the Linux acceptance to the independently downloaded and signature-checked
// release asset. Full M7 still checks its own Windows fixture independently.
func (a *App) portalLinuxCollectorCatalog(ctx context.Context, env *E2EEnvironment) (string, []string, error) {
	expected := env.State.Values["portal_linux_collector_sha"]
	if len(expected) != 64 {
		return "", nil, fmt.Errorf("portal Linux Collector requires the verified release asset digest")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", nil, err
	}
	distributions, err := client.JSONArray(ctx, "portal-linux-distributions", "enterprise", http.MethodGet, "/enterprise/telemetry/distributions", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return "", nil, err
	}
	target := strings.ReplaceAll(env.ImagePlatform, "/", "_")
	selected := ""
	for _, distribution := range distributions {
		if distribution["support_status"] != "supported" {
			continue
		}
		artifacts, _ := distribution["artifacts"].([]any)
		for _, value := range artifacts {
			artifact, _ := value.(map[string]any)
			if artifact["platform"] == target && artifact["sha256"] == expected {
				id, _ := distribution["id"].(string)
				if selected != "" && selected != id {
					return "", nil, fmt.Errorf("ambiguous verified Linux Collector release")
				}
				selected = id
			}
		}
	}
	if selected == "" {
		return "", nil, fmt.Errorf("the signed release Linux Collector is absent from the supported catalog")
	}
	profiles, err := client.JSONArray(ctx, "portal-linux-profiles", "enterprise", http.MethodGet, "/enterprise/telemetry/profiles", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return "", nil, err
	}
	ids := map[string]string{}
	for _, profile := range profiles {
		key, _ := profile["key"].(string)
		id, _ := profile["id"].(string)
		ids[key] = id
	}
	host := collectProfileIDs(ids, "host-basic", "linux-journald", "otlp-receiver")
	if len(host) != 3 {
		return "", nil, fmt.Errorf("Linux Collector profiles are incomplete")
	}
	raw, _ := json.MarshalIndent(map[string]any{"scope": "deployed Linux release asset; other platforms are outside this portal run", "distribution_id": selected, "platform": target, "sha256": expected, "profiles": host}, "", "  ")
	return selected, host, writePrivate(filepath.Join(env.Options.Artifacts, "portal-linux-collector-catalog.json"), raw)
}
