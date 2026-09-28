package argusctl

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestServerSoftMemoryBudgetFitsItsExistingContainerLimit(t *testing.T) {
	for _, profile := range []string{"evaluation", "local-hardening", "production"} {
		t.Run(profile, func(t *testing.T) {
			server := requireResource(t, resourcesByKind(renderPlatformResources(t, profile), "Deployment"), "argus-server")
			containers, _, err := unstructured.NestedSlice(server.Object, "spec", "template", "spec", "containers")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, raw := range containers {
				c := raw.(map[string]any)
				if c["name"] != "argus-server" {
					continue
				}
				limit, _, _ := unstructured.NestedString(c, "resources", "limits", "memory")
				if limit != "256Mi" {
					t.Fatalf("unexpected hard limit %s", limit)
				}
				env, _, _ := unstructured.NestedSlice(c, "env")
				for _, entry := range env {
					e := entry.(map[string]any)
					if e["name"] == "GOMEMLIMIT" {
						found = e["value"] == "192MiB"
					}
				}
			}
			if !found {
				t.Fatal("server has no soft memory headroom")
			}
		})
	}
}
