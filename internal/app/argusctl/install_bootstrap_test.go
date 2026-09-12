package argusctl

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestInstallerSharesBootstrapPolicyWithIngest(t *testing.T) {
	for _, profile := range []string{"evaluation", "local-hardening", "production"} {
		t.Run(profile, func(t *testing.T) {
			resources := renderPlatformResources(t, profile)
			config := requireResource(t, resourcesByKind(resources, "ConfigMap"), "argus-runtime-config")
			data, found, err := unstructured.NestedStringMap(config.Object, "data")
			if err != nil || !found {
				t.Fatal("missing Server/Worker runtime configuration")
			}
			ingest := requireResource(t, resourcesByKind(resources, "Deployment"), "argus-telemetry-ingest")
			containers, _, _ := unstructured.NestedSlice(ingest.Object, "spec", "template", "spec", "containers")
			container := containers[0].(map[string]any)
			env := map[string]string{}
			for _, raw := range container["env"].([]any) {
				item := raw.(map[string]any)
				value, _ := item["value"].(string)
				env[item["name"].(string)] = value
			}
			for _, name := range []string{"ARGUS_BOOTSTRAP_TLS_MODE", "ARGUS_TRUST_BUNDLE_PATH", "ARGUS_TRUST_BUNDLE_EPOCH"} {
				if data[name] == "" || env[name] != data[name] {
					t.Errorf("Ingest and Server/Worker disagree on %s", name)
				}
			}
		})
	}
}

func TestInstallerUsesOneInternalHTTPSAddress(t *testing.T) {
	resources := renderPlatformResources(t, "evaluation")
	config := requireResource(t, resourcesByKind(resources, "ConfigMap"), "argus-runtime-config")
	data, found, err := unstructured.NestedStringMap(config.Object, "data")
	if err != nil || !found {
		t.Fatal("missing Server/Worker runtime configuration")
	}
	const want = "ingress-nginx-controller.ingress-nginx.svc:443"
	for _, name := range []string{"ARGUS_ARTIFACT_INTERNAL_ADDRESS", "ARGUS_CONNECTOR_ENROLLMENT_FORWARD_TARGET"} {
		if got := data[name]; got != want {
			t.Errorf("%s = %q, want shared internal HTTPS address %q", name, got, want)
		}
	}
}
