package argusctl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testOpenBaoBootstrap(t *testing.T) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(map[string]any{
		"unseal_keys_b64": []string{base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))},
		"unseal_shares":   1, "unseal_threshold": 1, "root_token": "test-private-root-token-do-not-log",
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOpenBaoBackupRejectsMissingInvalidAndUnverifiedRecoveryMaterial(t *testing.T) {
	raw := testOpenBaoBootstrap(t)
	if err := validateOpenBaoBootstrap(raw); err != nil {
		t.Fatal(err)
	}
	for _, value := range [][]byte{nil, []byte(`{"root_token":"private"}`), bytes.Replace(raw, []byte(`"unseal_threshold": 1`), []byte(`"unseal_threshold": 2`), 1), bytes.Repeat([]byte("private"), 10000)} {
		if err := validateOpenBaoBootstrap(value); err == nil || strings.Contains(err.Error(), "test-private-root") {
			t.Fatal("invalid recovery state accepted or private material disclosed")
		}
	}
	stage := t.TempDir()
	manifest := backupManifest{FormatVersion: backupFormatVersion, ReleaseID: "backup-source", Files: map[string]string{}}
	for name, value := range map[string][]byte{openBaoBootstrapFile: raw, "openbao.snap": []byte("snapshot"), backupRecoveryFile: testBackupRecoveryJSON(t)} {
		if err := writePrivate(filepath.Join(stage, name), value); err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(filepath.Join(stage, name))
		if err != nil {
			t.Fatal(err)
		}
		manifest.Files[name] = digest
	}
	if err := validateBackupStage(stage, manifest); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{0, 1, 2, 4} {
		invalid := manifest
		invalid.FormatVersion = version
		if validateBackupStage(stage, invalid) == nil {
			t.Fatal("unsupported backup format accepted")
		}
	}
	for _, name := range []string{"../private", `..\private`, "C:private", "manifest.json"} {
		manifest.Files[name] = strings.Repeat("0", 64)
		if validateBackupStage(stage, manifest) == nil {
			t.Fatal("unsafe component name accepted")
		}
		delete(manifest.Files, name)
	}
	if err := os.WriteFile(filepath.Join(stage, openBaoBootstrapFile), append(raw, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if validateBackupStage(stage, manifest) == nil {
		t.Fatal("changed recovery state accepted without matching digest")
	}
	delete(manifest.Files, openBaoBootstrapFile)
	if validateBackupStage(stage, manifest) == nil {
		t.Fatal("unlisted recovery state accepted")
	}
}
