package argusctl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	backupFormatVersion  = 3
	openBaoBootstrapFile = "openbao-bootstrap.json"
	openBaoStatePath     = "/openbao/data/argus-bootstrap.json"
)

// Local-hardening uses a single Shamir share. Preserve the original JSON layout:
// the bootstrap Job and read-only unseal sidecar both consume this same file.
func validateOpenBaoBootstrap(raw []byte) error {
	var state struct {
		Keys      []string `json:"unseal_keys_b64"`
		Shares    int      `json:"unseal_shares"`
		Threshold int      `json:"unseal_threshold"`
		RootToken string   `json:"root_token"`
	}
	if len(raw) == 0 || len(raw) > 64<<10 || json.Unmarshal(raw, &state) != nil || len(state.Keys) != 1 || state.Shares != 1 || state.Threshold != 1 || len(state.RootToken) < 16 || strings.ContainsAny(state.RootToken, "\r\n\t ") {
		return errors.New("backup OpenBao recovery material is invalid")
	}
	key, err := base64.StdEncoding.DecodeString(state.Keys[0])
	if err != nil || len(key) != 32 {
		return errors.New("backup OpenBao recovery material is invalid")
	}
	return nil
}

func (a *App) readOpenBaoBootstrap(ctx context.Context, kube []string) ([]byte, error) {
	raw, err := a.runner.quiet(ctx, "kubectl", openBaoExecArgs(kube, "cat "+openBaoStatePath)...)
	if err != nil {
		// A partially successful read can contain key material in stderr/stdout.
		return nil, errors.New("read OpenBao recovery material failed; private output suppressed")
	}
	data := []byte(raw)
	if err := validateOpenBaoBootstrap(data); err != nil {
		return nil, err
	}
	return data, nil
}

func openBaoSnapshotRestoreCommand() string {
	return `state=` + openBaoStatePath + `; root=$(sed -n 's/.*"root_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$state"); BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$root" bao operator raft snapshot restore -force /tmp/argus-m8-restore.snap`
}

// Only the main container has a writable data mount. The sidecar stays read-only.
// Create beside the destination and rename so readers never see a partial key.
// The read-only unseal sidecar has another UID but shares Pod fsGroup 1000.
func openBaoBootstrapInstallCommand() string {
	return `umask 077
test -w /openbao/data
temporary=$(mktemp /openbao/data/.argus-bootstrap-restore.XXXXXX)
trap 'rm -f "$temporary"' EXIT
cat > "$temporary"
test -s "$temporary"
chmod 640 "$temporary"
mv -f "$temporary" ` + openBaoStatePath
}

func (a *App) installOpenBaoBootstrap(ctx context.Context, kube []string, raw []byte) error {
	if err := validateOpenBaoBootstrap(raw); err != nil {
		return err
	}
	args := append(append([]string{}, kube...), "exec", "-i", "statefulset/argus-openbao", "--container", "openbao", "--", "sh", "-ec", openBaoBootstrapInstallCommand())
	if _, err := a.runner.quietInput(ctx, bytes.NewReader(raw), "kubectl", args...); err != nil {
		return errors.New("install restored OpenBao recovery material failed; private output suppressed")
	}
	return nil
}

// Check component digests and mandatory recovery material before installing or
// writing to a restore target. Older formats lack complete platform trust/keys.
func validateBackupStage(stage string, manifest backupManifest) error {
	if manifest.FormatVersion != backupFormatVersion || manifest.Files[openBaoBootstrapFile] == "" || manifest.Files["openbao.snap"] == "" || manifest.Files[backupRecoveryFile] == "" {
		return errors.New("backup requires format 3 with OpenBao and platform recovery material")
	}
	for name, expected := range manifest.Files {
		if name == "." || name == ".." || name == "manifest.json" || strings.ContainsAny(name, `/\:`) || filepath.Base(name) != name {
			return errors.New("backup component name is invalid")
		}
		actual, err := fileSHA256(filepath.Join(stage, name))
		if err != nil || actual != expected {
			return errors.New("backup component failed integrity verification")
		}
	}
	raw, err := os.ReadFile(filepath.Join(stage, openBaoBootstrapFile))
	if err != nil {
		return errors.New("backup OpenBao recovery material is missing")
	}
	if err := validateOpenBaoBootstrap(raw); err != nil {
		return err
	}
	recovery, err := readBackupRecoveryState(stage)
	if err != nil {
		return err
	}
	if recovery.SourceReleaseID != manifest.ReleaseID {
		return errors.New("backup recovery source identity differs from the manifest")
	}
	return nil
}
