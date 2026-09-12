package directexecutor

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/kakj-go/Argus/internal/installation"
)

func TestLinuxDirectSSHTakeoverE2E(t *testing.T) {
	address := os.Getenv("ARGUS_TAKEOVER_E2E_SSH_ADDRESS")
	token := os.Getenv("ARGUS_TAKEOVER_E2E_TOKEN")
	connectorValue := os.Getenv("ARGUS_TAKEOVER_E2E_CONNECTOR_ID")
	artifactPath := os.Getenv("ARGUS_TAKEOVER_E2E_CONNECTOR_ARTIFACT")
	caPath := os.Getenv("ARGUS_TAKEOVER_E2E_CA_FILE")
	if address == "" || token == "" || connectorValue == "" || artifactPath == "" || caPath == "" {
		t.Skip("cross-cluster SSH takeover E2E is not configured")
	}
	connectorID, err := uuid.Parse(connectorValue)
	if err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.Open(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	defer binary.Close()
	expectedFingerprint := os.Getenv("ARGUS_TAKEOVER_E2E_HOST_KEY")
	if expectedFingerprint == "" {
		t.Fatal("ARGUS_TAKEOVER_E2E_HOST_KEY is required")
	}
	configuration := &ssh.ClientConfig{User: os.Getenv("ARGUS_TAKEOVER_E2E_SSH_USER"),
		Auth: []ssh.AuthMethod{ssh.Password(os.Getenv("ARGUS_TAKEOVER_E2E_SSH_PASSWORD"))}, Timeout: 10 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != expectedFingerprint {
				return errHostKeyMismatch
			}
			return nil
		}}
	client, err := ssh.Dial("tcp", address, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	plan := installation.HostConnectorInstallPlan{ConnectorID: connectorID, EnrollmentEndpoint: "https://argus.dev",
		TrustBundlePEM: caPEM, SigningPublicKey: os.Getenv("ARGUS_TAKEOVER_E2E_SIGNING_PUBLIC_KEY"),
		Artifact: installation.Artifact{SigningKeyID: "argus-collector-signing-v1"}, TargetPlatform: installation.LinuxAMD64}
	if err = installLinuxHostConnector(client, binary, plan, token); err != nil {
		t.Fatal(err)
	}
}
