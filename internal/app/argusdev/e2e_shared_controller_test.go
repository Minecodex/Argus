package argusdev

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/app/argusctl"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EOnlyReusesExplicitSharedControllerIdentity(t *testing.T) {
	cfg, err := argusctl.LoadConfig(filepath.Join("..", "..", "..", "deploy", "profiles", "evaluation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.OpenSandbox.SharedController = &argusctl.SharedSandboxController{Namespace: "other-project", ImageDigest: "sha256:" + strings.Repeat("a", 64)}
	path := filepath.Join(t.TempDir(), "install.json")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	pin, err := loadE2ESharedController(path)
	if err != nil || pin.Namespace != "other-project" || pin.ImageDigest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("explicit pin not retained: %v %+v", err, pin)
	}
	cfg.Spec.OpenSandbox.SharedController = nil
	raw, _ = json.Marshal(cfg)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadE2ESharedController(path); err == nil {
		t.Fatal("a config without a pin implicitly trusted the controller")
	}
}
