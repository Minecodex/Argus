package argusctl

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateHostValidationAndProbe(t *testing.T) {
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "evaluation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"https://cards.argus.dev", "argus.dev", "PLATFORM.ARGUS.DEV", "artifacts.argus.dev"} {
		cfg.Spec.Exposure.TemplateHost = host
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "templateHost") {
			t.Fatalf("unsafe template host %q: %v", host, err)
		}
	}
	cfg.Spec.Exposure.TemplateHost = "cards.argus.dev"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if webIngressHosts(cfg)[2] != "cards.argus.dev" {
		t.Fatal("verification must use the configured host")
	}
}

func TestTemplateHostRendersConsistentOriginAndCertificate(t *testing.T) {
	resources := renderPlatformResources(t, "evaluation", func(cfg *InstallConfig) { cfg.Spec.Exposure.TemplateHost = "cards.argus.dev" })
	config := requireResource(t, resourcesByKind(resources, "ConfigMap"), "argus-web-runtime-config")
	runtimeJSON := config.Object["data"].(map[string]any)["argus-runtime.json"].(string)
	if !strings.Contains(runtimeJSON, `"templateOrigin": "https://cards.argus.dev"`) {
		t.Fatal(runtimeJSON)
	}
	cert := requireResource(t, resourcesByKind(resources, "Certificate"), "argus-templates")
	names := cert.Object["spec"].(map[string]any)["dnsNames"].([]any)
	if len(names) != 1 || names[0] != "cards.argus.dev" {
		t.Fatalf("certificate DNS names: %v", names)
	}
	ingress := requireResource(t, resourcesByKind(resources, "Ingress"), "argus-web")
	found := false
	for _, raw := range ingress.Object["spec"].(map[string]any)["rules"].([]any) {
		host := raw.(map[string]any)["host"]
		if host == "templates.argus.dev" {
			t.Fatal("old host survived override")
		}
		if host == "cards.argus.dev" {
			found = true
		}
	}
	if !found {
		t.Fatal("template ingress missing")
	}
}
