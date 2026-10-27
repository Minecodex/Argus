package builtincollection

import (
	"github.com/google/uuid"
	"testing"
)

func TestFixedCollectionIdentityIncludesPhysicalResourceType(t *testing.T) {
	id := uuid.New()
	if StableID("host", id, "collector_metrics") == StableID("kubernetes_cluster", id, "collector_metrics") {
		t.Fatal("different authoritative resources shared a fixed instance")
	}
	for _, raw := range []string{`{"interval":"0s"}`, `{"interval":"2h"}`, `{"interval":"30s","endpoint":"http://foreign"}`} {
		if _, err := ParseFor("host_metrics", []byte(raw)); err == nil {
			t.Fatal("builtin accepted a remote endpoint or invalid interval")
		}
	}
	if value, err := ParseFor("host_metrics", nil); err != nil || value.Interval != "30s" {
		t.Fatal("default interval")
	}
}
