package argusdev

import (
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

func TestIsolatedIngressStartsBeforeSystemNamespaceExists(t *testing.T) {
	env := &E2EEnvironment{ReleaseID: "owned-run", IngressNS: "owned-ingress", IngressClass: "owned-class", SystemNS: "not-created-yet"}
	for _, document := range strings.Split(e2eIngressManifest(env), "---") {
		var deployment appsv1.Deployment
		if err := yaml.Unmarshal([]byte(document), &deployment); err != nil {
			t.Fatal(err)
		}
		if deployment.Kind != "Deployment" {
			continue
		}
		args := deployment.Spec.Template.Spec.Containers[0].Args
		for _, required := range []string{"--watch-namespace-selector=argus.io/release-id=owned-run", "--controller-class=k8s.io/owned-class", "--ingress-class=owned-class", "--publish-service=owned-ingress/argus-e2e-ingress"} {
			if !slices.Contains(args, required) {
				t.Fatalf("missing isolation argument %s: %v", required, args)
			}
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "--watch-namespace=") || arg == "--watch-ingress-without-class=true" {
				t.Fatalf("startup requires absent namespace or handles unrelated ingress: %s", arg)
			}
		}
		return
	}
	t.Fatal("ingress deployment not rendered")
}
