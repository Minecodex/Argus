package argusdev

import "testing"

func TestCoreRuntimeImageUsesLocalRegistryWithoutCleanupFiltering(t *testing.T) {
	for _, host := range []string{"host.docker.internal", "127.0.0.1", "localhost"} {
		image := host + ":55734/argus/argus-remote-runtime:owned@sha256:123"
		got, ok := localE2ERegistryImage(image)
		if !ok || got != "localhost:55734/argus/argus-remote-runtime:owned@sha256:123" {
			t.Fatal("core image failed local translation", host, got)
		}
	}
	for _, image := range []string{"alpine:3.21", "sha256:123", "docker.io/library/python:3.12", "other.example:5000/runtime:tag", "host.docker.internal:0/runtime:tag", "host.docker.internal:70000/runtime:tag"} {
		if got, ok := localE2ERegistryImage(image); ok || got != "" {
			t.Fatal("foreign or invalid image translated", image)
		}
	}
}
