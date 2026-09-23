package argusdev

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestP5FixturesKeepInstallationTrustWithoutOnboardingTargets(t *testing.T) {
	features := suiteFixtureFeatures("p5")
	if !features.Replay || !features.Artifact {
		t.Fatal("P5 requires a model fixture and the installation artifact signing root")
	}
	if features.SSH || features.Systemd || features.SystemdSidecar || features.P4Targets {
		t.Fatal("P5 unexpectedly depends on host onboarding fixtures")
	}
}

func TestP5CustomerFixtureCannotBecomeProtectedInfrastructure(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	env := &E2EEnvironment{Options: E2EOptions{Suite: "p5"}, ReleaseID: "p5-test", SystemNS: "p5-test-system", SandboxNS: "p5-test-sandbox", ObservNS: "p5-test-observability", Kube: &E2EKube{Client: client}}
	if err := prepareReplayFixtureNamespace(ctx, env); err != nil {
		t.Fatal(err)
	}
	name := env.ReplayNamespace()
	if name == env.SystemNS || name == env.SandboxNS || name == env.ObservNS {
		t.Fatal("customer MCP fixture is inside a protected infrastructure plane")
	}
	namespace, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil || namespace.Labels["argus.io/release-id"] != env.ReleaseID || len(env.ManagedNamespaces) != 1 {
		t.Fatal("fixture namespace ownership or cleanup registration is missing")
	}
	foreign := &E2EEnvironment{Options: env.Options, ReleaseID: env.ReleaseID, Kube: &E2EKube{Client: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})}}
	if err := prepareReplayFixtureNamespace(ctx, foreign); err == nil || len(foreign.ManagedNamespaces) != 0 {
		t.Fatal("existing namespace was adopted or registered for deletion")
	}
	for _, suite := range []string{"m4", "m7", "m10-query"} {
		other := &E2EEnvironment{Options: E2EOptions{Suite: suite}, SandboxNS: "existing-sandbox"}
		if other.ReplayNamespace() != other.SandboxNS {
			t.Fatal("unrelated suite fixture location changed")
		}
	}
}
