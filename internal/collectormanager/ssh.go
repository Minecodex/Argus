package collectormanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

type SSHOptions struct {
	Credential []byte
	Dial       func(context.Context) (net.Conn, error)
	Revalidate func(context.Context) error
}

func (manager Manager) ApplySSH(ctx context.Context, command *connectorv1.CollectorManagementCommand, options SSHOptions) (Result, error) {
	if err := Validate(command); err != nil {
		return Result{}, err
	}
	if command.GetResourceType() != "host" || command.GetTargetUsername() == "" || command.GetTargetPort() == 0 ||
		command.GetPinnedHostKey() == "" || len(options.Credential) == 0 || options.Dial == nil || options.Revalidate == nil {
		return Result{}, ErrInvalidCommand
	}
	auth, err := sshAuthentication(options.Credential)
	if err != nil {
		return Result{}, err
	}
	configuration := &ssh.ClientConfig{User: command.GetTargetUsername(), Auth: []ssh.AuthMethod{auth}, Timeout: 15 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != command.GetPinnedHostKey() {
				return ErrTargetHostKeyChanged
			}
			return nil
		}}
	connection, err := options.Dial(ctx)
	if err != nil {
		return Result{}, err
	}
	defer connection.Close()
	if err = options.Revalidate(ctx); err != nil {
		return Result{}, err
	}
	transport, channels, requests, err := ssh.NewClientConn(connection,
		net.JoinHostPort(command.GetTargetAddress(), fmt.Sprint(command.GetTargetPort())), configuration)
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			return Result{}, fmt.Errorf("%w: %w", ErrTargetAuthFailed, err)
		}
		return Result{}, err
	}
	client := ssh.NewClient(transport, channels, requests)
	defer client.Close()
	directory := "/var/lib/argus-otelcol/" + command.GetCollectorId()
	if command.GetOperation() == "uninstall" {
		fixed := "systemctl disable --now argus-otelcol.service >/dev/null 2>&1 || true; rm -rf /etc/argus-otelcol " + directory + " /etc/systemd/system/argus-otelcol.service; systemctl daemon-reload"
		if err = runSSH(client, fixed, nil); err != nil {
			return Result{}, err
		}
		return sshResult(command, "uninstalled"), nil
	}
	artifactFile, err := os.CreateTemp("", ".argus-collector-artifact-*")
	if err != nil {
		return Result{}, err
	}
	artifactPath := artifactFile.Name()
	defer func() {
		_ = artifactFile.Close()
		_ = os.Remove(artifactPath)
	}()
	if err = artifactFile.Chmod(0o600); err != nil {
		return Result{}, err
	}
	if err = manager.FetchArtifactTo(ctx, command.GetArtifact(), artifactFile); err != nil {
		return Result{}, err
	}
	if _, err = artifactFile.Seek(0, io.SeekStart); err != nil {
		return Result{}, err
	}
	directory += "/candidate-" + strconv.FormatUint(command.GetDesiredRevision(), 10) + "-" + uuid.NewString()
	prepare := "umask 077; install -d -m 0700 " + directory + "/release; cat > " + directory + "/artifact.tar.gz"
	if err = runSSHReader(client, prepare, artifactFile); err != nil {
		return Result{}, err
	}
	defer func() { _ = runSSH(client, "rm -rf "+directory, nil) }()
	runtimeConfig, err := configbundle.Extract(command.GetRenderedConfig(), "host")
	if err != nil {
		return Result{}, err
	}
	private, err := monitoringPrivateConfig(command)
	if err != nil {
		return Result{}, err
	}
	defer clear(private)
	trust, err := commandTrustBundle(command)
	if err != nil {
		return Result{}, err
	}
	files := map[string][]byte{"config.json": runtimeConfig, "server-ca.pem": trust.PEM, "service.unit": []byte(localCollectorSystemdUnit(command, "/var/lib/argus-otelcol")), "revision": []byte(strconv.FormatUint(command.GetDesiredRevision(), 10) + "\n"), "config-hash": []byte(command.GetConfigSha256() + "\n"), "collector-id": []byte(command.GetCollectorId() + "\n")}
	for name, value := range files {
		if err = runSSH(client, "umask 077; cat > "+directory+"/"+name, value); err != nil {
			return Result{}, err
		}
	}
	var payload bytes.Buffer
	archive := tar.NewWriter(&payload)
	for _, file := range []struct {
		name  string
		value []byte
	}{{"private.json", private}, {"enrollment-token", command.GetEnrollmentToken()}} {
		if err = archive.WriteHeader(&tar.Header{Name: file.name, Mode: 0600, Size: int64(len(file.value)), Typeflag: tar.TypeReg}); err != nil {
			return Result{}, err
		}
		if _, err = archive.Write(file.value); err != nil {
			return Result{}, err
		}
	}
	if err = archive.Close(); err != nil {
		return Result{}, err
	}
	defer clear(payload.Bytes())
	if err = runSSH(client, sshActivateCommand(directory), payload.Bytes()); err != nil {
		var remote *ssh.ExitError
		if errors.As(err, &remote) {
			switch remote.ExitStatus() {
			case 85:
				return Result{}, ErrInvalidCommand
			case 86:
				return Result{}, ErrConfigInvalid
			case 87:
				return Result{}, ErrRollbackFailed
			}
		}
		return Result{}, err
	}
	return sshResult(command, "converged"), nil
}

func sshAuthentication(value []byte) (ssh.AuthMethod, error) {
	if bytes.Contains(value, []byte("PRIVATE KEY")) {
		signer, err := ssh.ParsePrivateKey(value)
		if err != nil {
			return nil, err
		}
		return ssh.PublicKeys(signer), nil
	}
	return ssh.Password(string(value)), nil
}

func runSSH(client *ssh.Client, command string, input []byte) error {
	if input == nil {
		return runSSHReader(client, command, nil)
	}
	return runSSHReader(client, command, bytes.NewReader(input))
}

func runSSHReader(client *ssh.Client, command string, input io.Reader) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	if input != nil {
		session.Stdin = input
	}
	// 远端 stderr 必须进入错误信息:目标缺命令(exit 127)、权限不足等
	// 失败原因只有远端输出能说明,仅退出码无法诊断。
	var stderr bytes.Buffer
	session.Stderr = &stderr
	if err = session.Run(command); err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
}

func sshResult(command *connectorv1.CollectorManagementCommand, status string) Result {
	diagnostic := sha256.Sum256([]byte(strings.Join([]string{command.GetCollectorId(), status, command.GetConfigSha256()}, "\x00")))
	return Result{CollectorID: command.GetCollectorId(), EffectiveRevision: command.GetDesiredRevision(), ConfigSHA256: strings.ToLower(command.GetConfigSha256()),
		Status: status, DiagnosticHash: hex.EncodeToString(diagnostic[:])}
}
