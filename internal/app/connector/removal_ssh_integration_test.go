package connector

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBastionSSHRemovalIntegration(t *testing.T) {
	if os.Getenv("ARGUS_REMOVAL_INTEGRATION") != "1" {
		t.Skip("set ARGUS_REMOVAL_INTEGRATION=1 for disposable SSH execution")
	}
	docker := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker: %v %s", err, out)
		}
		return string(out)
	}
	name := "argus-bastion-removal-test-" + uuid.NewString()[:8]
	docker("run", "-d", "--privileged", "--name", name, "--tmpfs", "/run", "--tmpfs", "/run/lock", "-p", "127.0.0.1::22", "argus-systemd-host")
	t.Cleanup(func() { docker("rm", "-f", name) })
	for i := 0; i < 40; i++ {
		if exec.Command("docker", "exec", name, "systemctl", "is-active", "--quiet", "ssh").Run() == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	binary := filepath.Join(t.TempDir(), "argus-connector")
	root, _ := filepath.Abs("../../..")
	build := exec.Command("go", "build", "-o", binary, "./cmd/argus-connector")
	build.Dir = root
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	docker("cp", binary, name+":/usr/local/bin/argus-connector")
	op, connector, host := uuid.New(), uuid.New(), uuid.New()
	docker("exec", name, "sh", "-ec", `mkdir -p /var/lib/argus-connector /etc/argus-connector; chmod 755 /usr/local/bin/argus-connector; printf '{"connector_id":"`+connector.String()+`"}' >/var/lib/argus-connector/identity.json; printf '[Service]\nExecStart=/bin/sleep infinity\n' >/etc/systemd/system/argus-connector.service; systemctl daemon-reload; systemctl start argus-connector`)
	password := uuid.NewString()
	cmd := exec.Command("docker", "exec", "-i", name, "chpasswd")
	cmd.Stdin = strings.NewReader("root:" + password + "\n")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(docker("port", name, "22/tcp"))
	ip, portValue, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portValue)
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(docker("exec", name, "cat", "/etc/ssh/ssh_host_ecdsa_key.pub")))
	if err != nil {
		t.Fatal(err)
	}
	request := &connectorv1.HostConnectorRemoval{OperationId: op.String(), HostId: host.String(), ConnectorId: connector.String(), RemovalGeneration: 1, TargetPlatform: "linux_amd64", Address: ip, Port: uint32(port), Username: "root", PinnedHostKey: ssh.FingerprintSHA256(key)}
	bad := proto.Clone(request).(*connectorv1.HostConnectorRemoval)
	bad.ConnectorId = uuid.NewString()
	typed, _ := anypb.New(bad)
	if _, err = executeHostConnectorRemoval(context.Background(), typed, []byte(password)); err == nil || !strings.Contains(err.Error(), "TARGET_IDENTITY_CHANGED") {
		t.Fatalf("machine identity fence failed: %v", err)
	}
	docker("exec", name, "systemctl", "is-active", "--quiet", "argus-connector")
	typed, _ = anypb.New(request)
	result, err := executeHostConnectorRemoval(context.Background(), typed, []byte(password))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = hostremoval.ParseCleanupEvidence(result.CleanupEvidenceJson, op, connector, 1); err != nil {
		t.Fatal(err)
	}
	retry, err := executeHostConnectorRemoval(context.Background(), typed, []byte(password))
	if err != nil {
		t.Fatalf("SSH replay after Connector removal: %v", err)
	}
	var original, replayed hostremoval.CleanupEvidence
	_ = json.Unmarshal(result.CleanupEvidenceJson, &original)
	_ = json.Unmarshal(retry.CleanupEvidenceJson, &replayed)
	if original != replayed {
		t.Fatal("retry changed cleanup evidence")
	}
	docker("exec", name, "sh", "-ec", "test ! -e /usr/local/bin/argus-connector; systemctl is-active --quiet ssh")
}
