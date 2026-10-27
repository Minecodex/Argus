package argusctl

import (
	"strings"
	"testing"
)

func TestCRDHookUsesFrozenDedicatedInstallationTools(t *testing.T) {
	for _, repository := range []string{"registry.example/argus/argus-install-tools", "host.docker.internal:5001/argus/argus-install-tools"} {
		ref := repository + ":verified-build@sha256:" + strings.Repeat("a", 64)
		repo, tag, err := pinnedCRDHookImage(ref)
		if err != nil || repo+":"+tag != ref {
			t.Fatal("hook lost immutable image identity", err)
		}
	}
	for _, ref := range []string{"bitnami/kubectl:latest", "runtime:version", "runtime@sha256:" + strings.Repeat("a", 64), "runtime:version@sha256:bad", "registry.example/argus/argus-remote-runtime:version@sha256:" + strings.Repeat("a", 64)} {
		if _, _, err := pinnedCRDHookImage(ref); err == nil {
			t.Fatal("hook accepted an unfrozen or malformed image")
		}
	}
}
