package argusdev

import (
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestP5SandboxInstallationTransitionPreservesIdentityAndStorage(t *testing.T) {
	input := []byte(`spec:
  releaseId: p5-owned
  profile: evaluation
  images: {tag: immutable-fixture, registry: registry.example}
  workspace: {enabled: true, storageClass: argus-workspace, defaultBytes: 268435456}
  pki: {mode: managed}
  openSandbox: {enabled: false, namespace: p5-owned-sandbox}
`)
	updated, err := p5SandboxInstallConfig(input, "p5-owned")
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	if err := yaml.Unmarshal(input, &before); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(updated, &after); err != nil {
		t.Fatal(err)
	}
	nestedMap(nestedMap(before, "spec"), "openSandbox")["enabled"] = true
	if !reflect.DeepEqual(before, after) {
		t.Fatal("transition changed more than the optional compute capability")
	}
	if _, err := p5SandboxInstallConfig(updated, "p5-owned"); err == nil {
		t.Fatal("already-enabled runtime hid missing agent-lite phase")
	}
	if _, err := p5SandboxInstallConfig(input, "another-release"); err == nil {
		t.Fatal("foreign release configuration accepted")
	}
}
