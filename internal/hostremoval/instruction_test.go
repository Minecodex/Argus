package hostremoval

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestLinuxRemovalScriptIsStrictAndOrdered(t *testing.T) {
	plan := testPlan("linux_amd64")
	script := buildRemovalScript(plan, "https://argus.example/api/v1/host-removal/receipt", "receipt-token")
	if strings.Contains(script, "--insecure") || !strings.Contains(script, `--cacert "$JOURNAL/server-ca.pem"`) {
		t.Fatal("removal receipt must use the installed Trust Bundle without TLS bypass")
	}
	copyHelper := strings.Index(script, "cp /usr/local/bin/argus-connector")
	helper := strings.Index(script, "uninstall-local")
	verification := strings.Index(script, `"evidence":%s`)
	receipt := strings.Index(script, "X-Argus-Removal-Token")
	if copyHelper < 0 || helper <= copyHelper || verification <= helper || receipt <= verification {
		t.Fatalf("unexpected removal order: copy=%d helper=%d verification=%d receipt=%d", copyHelper, helper, verification, receipt)
	}
	if strings.Contains(script, "%!") {
		t.Fatalf("removal script contains an invalid format expansion: %s", script)
	}
	if strings.Contains(script, "\r") {
		t.Fatal("POSIX removal script must use LF line endings")
	}
	if strings.Contains(script, "sshd") || strings.Contains(script, "openssh") {
		t.Fatal("removal must not modify OpenSSH")
	}
}

func TestWindowsRemovalRestoresOnlyRecordedRDPChanges(t *testing.T) {
	plan := testPlan("windows_amd64")
	plan.ManagedChangeID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	plan.ManagedChangeBefore = json.RawMessage(`{"f_deny_ts_connections":1,"user_authentication":0,"firewall_tcp_enabled":false,"firewall_udp_enabled":false,"term_service_running":false}`)
	plan.ManagedChangeApplied = json.RawMessage(`{"f_deny_ts_connections":0,"user_authentication":1,"firewall_tcp_enabled":true,"firewall_udp_enabled":true,"term_service_running":true}`)
	script := buildRemovalScript(plan, "https://argus.example/api/v1/host-removal/receipt", "receipt-token")
	for _, expected := range []string{"$currentDeny -eq $applied.f_deny_ts_connections", "$currentNla -eq $applied.user_authentication", "$drift=$true", "$rdpStatus='drifted'", "argus-uninstaller.exe", "uninstall-local", "evidence=$evidence"} {
		if !strings.Contains(script, expected) {
			t.Fatalf("Windows removal script does not contain %q", expected)
		}
	}
	if strings.Contains(script, "--insecure") || strings.Contains(strings.ToLower(script), "openssh") {
		t.Fatal("Windows removal must keep strict TLS and preserve OpenSSH")
	}
	if powershell, err := exec.LookPath("powershell.exe"); err == nil {
		command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", `[void][ScriptBlock]::Create([Console]::In.ReadToEnd())`)
		command.Stdin = bytes.NewBufferString(script)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("PowerShell removal script does not parse: %v: %s", err, output)
		}
	}
	if strings.Contains(script, "%!") {
		t.Fatalf("removal script contains an invalid format expansion: %s", script)
	}
}

func TestCleanupEvidenceBindsObservedPostconditions(t *testing.T) {
	operationID, connectorID := uuid.New(), uuid.New()
	evidence := CleanupEvidence{SchemaVersion: CleanupEvidenceSchema, OperationID: operationID, ConnectorID: connectorID,
		RemovalGeneration: 4, Platform: "linux", CollectorServiceAbsent: true, CollectorProcessAbsent: true, CollectorFilesAbsent: true,
		ConnectorServiceAbsent: true, ConnectorProcessAbsent: true, ConnectorFilesAbsent: true, ConnectorUserAbsent: true, RelayPortsReleased: true,
		RDPConfigStatus: "not_applicable", ObservedAt: time.Unix(1_700_000_000, 0).UTC()}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	_, canonical, err := ParseCleanupEvidence(raw, operationID, connectorID, 4)
	if err != nil || len(CleanupEvidenceDigest(canonical)) != sha256.Size {
		t.Fatalf("valid cleanup evidence was rejected: %v", err)
	}
	evidence.ConnectorFilesAbsent = false
	raw, _ = json.Marshal(evidence)
	if _, _, err = ParseCleanupEvidence(raw, operationID, connectorID, 4); err == nil {
		t.Fatal("an unsatisfied cleanup postcondition must be rejected")
	}
}

func TestParseSSHCleanupOutputRejectsPredictableSuccessHash(t *testing.T) {
	operationID, connectorID := uuid.New(), uuid.New()
	if _, _, _, err := ParseSSHCleanupOutput("ARGUS_CLEANUP_EVIDENCE_BASE64="+base64.StdEncoding.EncodeToString([]byte(`{"operation_id":"`+operationID.String()+`"}`)), operationID, connectorID, 1); err == nil {
		t.Fatal("an operation identifier without observed cleanup evidence must be rejected")
	}
}

func TestDownloadCommandsPinScriptAndUseStrictTLS(t *testing.T) {
	for _, platform := range []string{"linux_amd64", "windows_amd64"} {
		plan := testPlan(platform)
		plan.HTTPSDialAddress = "10.0.0.8:8445"
		command, _, _ := buildDownloadCommand(plan, "https://argus.example/api/v1/host-removal/bootstrap-script?operation_id="+plan.OperationID.UUID.String(), "bootstrap-token", strings.Repeat("a", 64))
		if strings.Contains(command, "--insecure") || !strings.Contains(command, "--cacert") || !strings.Contains(command, "--connect-to") || !strings.Contains(strings.ToLower(command), "sha256") {
			t.Fatalf("%s removal command is not fail-closed: %s", platform, command)
		}
	}
}

func TestRemovalTokenEnvelopeRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	enterpriseID, operationID := uuid.New(), uuid.New()
	token, digest, err := newToken()
	if err != nil || len(digest) != sha256.Size {
		t.Fatal(err)
	}
	nonce, ciphertext, err := sealToken(key, enterpriseID, operationID, tokenEnvelope{Token: token, ReceiptToken: "receipt"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := openToken(key, nonce, ciphertext, enterpriseID, operationID)
	if err != nil || value.Token != token || value.ReceiptToken != "receipt" {
		t.Fatalf("token envelope round trip failed: %+v, %v", value, err)
	}
	if _, err = openToken(key, nonce, ciphertext, enterpriseID, uuid.New()); err == nil {
		t.Fatal("token envelope must be bound to the operation")
	}
}

func TestDecodeOperationPlanRejectsMutation(t *testing.T) {
	plan := testPlan("linux_arm64")
	encoded, _ := canonicalPlan(plan)
	digest := sha256.Sum256(encoded)
	operation := db.HostRemovalOperation{ID: plan.OperationID.UUID, HostID: plan.HostID, ConnectorID: plan.ConnectorID,
		RemovalGeneration: plan.RemovalGeneration, Plan: encoded, PlanHash: digest[:]}
	if _, err := DecodeOperationPlan(operation); err != nil {
		t.Fatal(err)
	}
	operation.Plan = append([]byte(nil), encoded...)
	operation.Plan[len(operation.Plan)-2] ^= 1
	if _, err := DecodeOperationPlan(operation); err == nil {
		t.Fatal("mutated operation plan must be rejected")
	}
}

func testPlan(platform string) Plan {
	operationID := uuid.New()
	return Plan{SchemaVersion: "argus.host_removal/v1", OperationID: uuid.NullUUID{UUID: operationID, Valid: true},
		TargetType: TargetManagedHost, TargetID: uuid.New(), HostID: uuid.New(), ConnectorID: uuid.New(),
		RemovalGeneration: 3, TargetPlatform: platform, DeliveryMethod: "manual", SSHPath: "none", TrustBundleEpoch: 1}
}
