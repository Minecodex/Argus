package argusdev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

type plan8ViewMaterial struct {
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}
type plan8VerificationMaterials struct {
	Source           string                       `json:"source_fingerprint"`
	Distribution     string                       `json:"distribution_version"`
	CollectorVersion string                       `json:"collector_version"`
	Collectors       map[string]string            `json:"collector_platform_sha256"`
	Views            map[string]plan8ViewMaterial `json:"views"`
}

func (a *App) plan8Materials(source plan8SourceSnapshot) (plan8VerificationMaterials, error) {
	m := plan8VerificationMaterials{Source: source.Fingerprint, Distribution: configbundle.DistributionVersion, CollectorVersion: configbundle.CollectorVersion, Collectors: map[string]string{}, Views: map[string]plan8ViewMaterial{}}
	paths := map[string]string{"build/otelcol/dist/linux-amd64/argus-otelcol": "linux_amd64", "build/otelcol/dist/linux-arm64/argus-otelcol": "linux_arm64", "build/otelcol/dist/windows-amd64/argus-otelcol.exe": "windows_amd64"}
	for _, f := range source.Files {
		if platform := paths[f.Path]; platform != "" && len(f.SHA256) == 64 {
			m.Collectors[platform] = f.SHA256
		}
	}
	body, err := os.ReadFile(filepath.Join(a.root, "api", "contracts", "dashboard-templates.json"))
	if err != nil {
		return m, err
	}
	var entries []map[string]any
	if json.Unmarshal(body, &entries) != nil {
		return m, errors.New("managed template materials unavailable")
	}
	for _, entry := range entries {
		key, ok := entry["key"].(string)
		version, number := entry["version"].(float64)
		if !ok || !number || version < 1 || float64(int64(version)) != version {
			return m, errors.New("managed template identity unavailable")
		}
		canonical, err := json.Marshal(entry)
		if err != nil {
			return m, err
		}
		digest := sha256.Sum256(canonical)
		m.Views[key] = plan8ViewMaterial{Version: int64(version), SHA256: hex.EncodeToString(digest[:])}
	}
	// Shared APM/log entry points are actual query contracts, rather than
	// invented independent Grafana templates for every ingestion protocol.
	for key, path := range map[string]string{"application.apm": "api/contracts/application-apm-queries.json", "logs.explorer": "api/openapi/components/m7.yaml", "release.bundle": "api/contracts/dashboard-templates.json"} {
		body, err := os.ReadFile(filepath.Join(a.root, filepath.FromSlash(path)))
		if err != nil {
			return m, err
		}
		digest := sha256.Sum256(body)
		m.Views[key] = plan8ViewMaterial{Version: 1, SHA256: hex.EncodeToString(digest[:])}
	}
	return m, nil
}

func (m plan8VerificationMaterials) validate(platform, distribution, collectorVersion, collectorSHA, view string, viewVersion int64, viewSHA string) error {
	if m.Collectors[platform] == "" || m.Collectors[platform] != collectorSHA || distribution != m.Distribution || collectorVersion != m.CollectorVersion {
		return errors.New("matrix Collector does not match the exact frozen platform artifact")
	}
	v, ok := m.Views[view]
	if !ok || v.Version != viewVersion || v.SHA256 != viewSHA {
		return errors.New("matrix default view does not match its frozen definition")
	}
	return nil
}
