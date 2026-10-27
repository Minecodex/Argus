package logcollection

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestHostFileLogKeepsKubernetesContainerPathsInDedicatedEntry(t *testing.T) {
	for _, path := range []string{"/var/log/pods/orders_abc/main/0.log", "/var/log/containers/orders_default_main-abc.log", "/var/log/pods/**/*.log", "/var/log/containers/*.log"} {
		raw, _ := json.Marshal(map[string]any{"scope": "system", "include": []string{path}})
		if _, e := Normalize("filelog", "linux_amd64", raw); !errors.Is(e, ErrInvalid) {
			t.Fatalf("host file rule admitted Kubernetes path %s: %v", path, e)
		}
	}
	for _, path := range []string{"/var/log/orders/*.log", "/var/log/pods-other/*.log", "/var/log/**"} {
		raw, _ := json.Marshal(map[string]any{"scope": "system", "include": []string{path}})
		if _, e := Normalize("filelog", "linux_amd64", raw); e != nil {
			t.Fatalf("ordinary/broad host rule refused: %s %v", path, e)
		}
	}
	raw := []byte(`{"scope":"system","include":["C:/var/log/pods/*.log"]}`)
	if _, e := Normalize("filelog", "windows_amd64", raw); e != nil {
		t.Fatal("Linux ownership rule was applied to Windows", e)
	}
}
