package connector

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/installation"
)

func TestWindowsBastionSSHInstallCommitsEnrollmentBeforeTakeover(t *testing.T) {
	plan := installation.HostConnectorInstallPlan{ConnectorID: uuid.New(), EnrollmentEndpoint: "https://argus.example.test",
		EnrollDialAddress: "127.0.0.1:8445", GatewayDialAddress: "127.0.0.1:9445"}
	script := buildWindowsBastionTakeoverScript(plan, `C:\ProgramData\Argus\Install\`+plan.ConnectorID.String())
	position := -1
	for _, expected := range []string{"& $next enroll", "Stop-Service $serviceName", "Copy-Item -LiteralPath $next", "Start-Service ArgusConnector"} {
		next := strings.Index(script, expected)
		if next <= position {
			t.Fatalf("takeover script order is invalid for %q: %s", expected, script)
		}
		position = next
	}
	if strings.Contains(script, "--insecure") || !strings.Contains(script, "previous-connector-id") {
		t.Fatal("takeover script omitted strict staging semantics")
	}
	if powershell, err := exec.LookPath("powershell.exe"); err == nil {
		command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", `[void][ScriptBlock]::Create([Console]::In.ReadToEnd())`)
		command.Stdin = bytes.NewBufferString(script)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("PowerShell takeover script does not parse: %v: %s", err, output)
		}
	}
}
