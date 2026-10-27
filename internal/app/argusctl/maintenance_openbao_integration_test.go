package argusctl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Exercises the production recovery scripts, encrypted archive and non-root
// atomic state replacement against two independently initialized real servers.
func TestLiveOpenBaoEncryptedBackupRestoresSourceKeyAfterRestart(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_OPENBAO_RECOVERY") != "1" {
		t.Skip("explicit isolated OpenBao recovery integration required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	owner := "argus-bao-recovery-" + uuid.NewString()[:8]
	command := func(input []byte, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdin = bytes.NewReader(input)
		// Do not attach private subprocess output to test failures.
		return cmd.Output()
	}
	must := func(input []byte, args ...string) []byte {
		t.Helper()
		output, err := command(input, args...)
		if err != nil {
			t.Fatalf("owned OpenBao operation failed (%s); private output suppressed", args[0])
		}
		return output
	}
	containerExec := func(id string, input []byte, args ...string) []byte {
		t.Helper()
		return must(input, append([]string{"exec", "-i", id}, args...)...)
	}
	privateCLI := func(id, token string, args ...string) []byte {
		t.Helper()
		return containerExec(id, []byte(token+"\n"), append([]string{"sh", "-ec", `read -r token; export BAO_TOKEN="$token"; exec "$@"`, "private-command", "bao"}, args...)...)
	}
	waitSealed := func(id string) {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
			output, _ := command(nil, "exec", id, "bao", "status", "-format=json")
			var status map[string]any
			if json.Unmarshal(output, &status) == nil && status["initialized"] == true && status["sealed"] == true {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatal("restored OpenBao did not reach initialized sealed state")
	}
	var ids []string
	for _, suffix := range []string{"source", "target"} {
		config, _ := json.Marshal(map[string]any{
			"disable_mlock": true, "api_addr": "http://127.0.0.1:8200", "cluster_addr": "http://127.0.0.1:8201",
			"listener": map[string]any{"tcp": map[string]any{"address": "127.0.0.1:8200", "tls_disable": true}},
			"storage":  map[string]any{"raft": map[string]any{"path": "/openbao/data/raft", "node_id": suffix}},
		})
		id := strings.TrimSpace(string(must(nil, "run", "--detach", "--name", owner+"-"+suffix,
			"--label", "argus.owner="+owner, "--network=none", "--memory=256m", "--cpus=0.5",
			"--mount", "type=volume,destination=/openbao/data", "--env", "BAO_ADDR=http://127.0.0.1:8200",
			"--env", "BAO_LOCAL_CONFIG="+string(config), "--entrypoint", "sh", "openbao/openbao:2.3.2", "-ec",
			`mkdir -p /openbao/data/raft; chown 1000:1000 /openbao/data /openbao/data/raft; chmod 770 /openbao/data; printf '%s' "$BAO_LOCAL_CONFIG" >/tmp/bao.json; chmod 644 /tmp/bao.json; exec su-exec 1000:1000 bao server -config=/tmp/bao.json`)))
		ids = append(ids, id)
		t.Cleanup(func() {
			clean, stop := context.WithTimeout(context.Background(), 30*time.Second)
			defer stop()
			output, err := exec.CommandContext(clean, "docker", "inspect", "--format", `{{.Id}} {{index .Config.Labels "argus.owner"}}`, id).Output()
			if err != nil || strings.TrimSpace(string(output)) != id+" "+owner {
				t.Error("refuse cleanup of changed OpenBao fixture")
				return
			}
			if err := exec.CommandContext(clean, "docker", "rm", "--force", "--volumes", id).Run(); err != nil {
				t.Error("owned OpenBao fixture cleanup failed")
			}
		})
		ready := false
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
			output, _ := command(nil, "exec", id, "bao", "status", "-format=json")
			var status map[string]any
			if json.Unmarshal(output, &status) == nil && status["initialized"] == false {
				ready = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !ready {
			t.Fatal("owned OpenBao fixture not ready")
		}
	}
	var states [][]byte
	var tokens, keys []string
	for _, id := range ids {
		raw := containerExec(id, nil, "bao", "operator", "init", "-key-shares=1", "-key-threshold=1", "-format=json")
		if err := validateOpenBaoBootstrap(raw); err != nil {
			t.Fatal(err)
		}
		var state struct {
			Keys  []string `json:"unseal_keys_b64"`
			Token string   `json:"root_token"`
		}
		if json.Unmarshal(raw, &state) != nil {
			t.Fatal("invalid fixture state")
		}
		states = append(states, raw)
		tokens, keys = append(tokens, state.Token), append(keys, state.Keys[0])
		containerExec(id, []byte(state.Keys[0]+"\n"), "sh", "-ec", `read -r key; bao operator unseal "$key" >/dev/null`)
		containerExec(id, raw, "sh", "-ec", "umask 077; cat > "+openBaoStatePath)
	}
	source, target := ids[0], ids[1]
	privateCLI(source, tokens[0], "secrets", "enable", "transit")
	privateCLI(source, tokens[0], "write", "-f", "transit/keys/recovery")
	plain := base64.StdEncoding.EncodeToString([]byte(uuid.NewString()))
	sealed := privateCLI(source, tokens[0], "write", "-format=json", "transit/encrypt/recovery", "plaintext="+plain)
	var encrypted struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if json.Unmarshal(sealed, &encrypted) != nil || encrypted.Data.Ciphertext == "" {
		t.Fatal("fixture transit encryption failed")
	}
	containerExec(source, nil, "sh", "-ec", openBaoSnapshotSaveCommand("/tmp/argus-m8.snap"))
	snapshot := containerExec(source, nil, "cat", "/tmp/argus-m8.snap")
	stage, backupDir := t.TempDir(), t.TempDir()
	manifest := backupManifest{FormatVersion: backupFormatVersion, ReleaseID: "backup-source", Files: map[string]string{}}
	for name, value := range map[string][]byte{openBaoBootstrapFile: states[0], "openbao.snap": snapshot, backupRecoveryFile: testBackupRecoveryJSON(t)} {
		if err := writePrivate(filepath.Join(stage, name), value); err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(filepath.Join(stage, name))
		if err != nil {
			t.Fatal(err)
		}
		manifest.Files[name] = digest
	}
	data, _ := json.Marshal(manifest)
	if err := writePrivate(filepath.Join(stage, "manifest.json"), data); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(backupDir, "recovery.argusbak")
	if err := encryptDirectory(stage, archive, key); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(archive+".key", []byte(base64.RawURLEncoding.EncodeToString(key))); err != nil {
		t.Fatal(err)
	}
	restored, _, err := openBackup(archive, "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(restored)
	recovered, err := os.ReadFile(filepath.Join(restored, openBaoBootstrapFile))
	if err != nil || !bytes.Equal(recovered, states[0]) {
		t.Fatal("encrypted source recovery material changed")
	}
	containerExec(target, snapshot, "sh", "-ec", "umask 077; cat > /tmp/argus-m8-restore.snap")
	containerExec(target, nil, "sh", "-ec", openBaoSnapshotRestoreCommand())
	// Reproduce the broken path before applying the fix: a fresh destination's
	// own key cannot unseal the source snapshot after a real restart.
	must(nil, "restart", target)
	waitSealed(target)
	if _, err := command([]byte(keys[1]+"\n"), "exec", "-i", target, "sh", "-ec", `read -r key; bao operator unseal "$key" >/dev/null`); err == nil {
		t.Fatal("independent target key unexpectedly opened source snapshot")
	}
	// The production main container is UID 1000; the unseal sidecar is read-only.
	must(recovered, "exec", "-i", "--user", "1000:1000", target, "sh", "-ec", openBaoBootstrapInstallCommand())
	mode := strings.TrimSpace(string(containerExec(target, nil, "stat", "-c", "%a", openBaoStatePath)))
	if mode != "640" {
		t.Fatal("recovery material is not limited to its owner and shared read group")
	}
	if _, err := command(nil, "exec", "--user", "100:100", target, "test", "-r", openBaoStatePath); err == nil {
		t.Fatal("unrelated reader could read recovery material")
	}
	must(nil, "exec", "--user", "100:1000", target, "test", "-r", openBaoStatePath)
	if _, err := command(nil, "exec", "--user", "100:1000", target, "test", "-w", openBaoStatePath); err == nil {
		t.Fatal("shared group reader could modify recovery material")
	}
	must(nil, "restart", target)
	waitSealed(target)
	must(nil, "exec", "--user", "100:1000", target, "sh", "-ec", `state=`+openBaoStatePath+`; key=$(awk '/"unseal_keys_b64"/ {getline; gsub(/[", ]/, ""); print; exit}' "$state"); test -n "$key"; bao operator unseal "$key" >/dev/null`)
	opened := privateCLI(target, tokens[0], "write", "-format=json", "transit/decrypt/recovery", "ciphertext="+encrypted.Data.Ciphertext)
	var decrypted struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if json.Unmarshal(opened, &decrypted) != nil || decrypted.Data.Plaintext != plain {
		t.Fatal("restored source transit key could not decrypt original data")
	}
	t.Log("source snapshot and encrypted recovery state restored; unrelated target key and reader rejected; UID 1000 atomic write; different UID shared-group read-only reader unsealed after restart; original transit ciphertext passed")
}
