package installinstruction

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/installation"
)

func TestBuildPOSIXSerializesEmptyWarningsAsArray(t *testing.T) {
	result, err := BuildPOSIX(POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxAMD64,
		InstallerURL: "https://artifacts.example.com/install.sh", InstallerSHA256: strings.Repeat("a", 64),
		BootstrapScriptURL: "https://argus.example.com/api/v1/connectors/bootstrap-script", DownloadTLSMode: DownloadTLSStrict,
		TrustBundlePEM: testBundle(t), TrustBundleEpoch: 1, Token: "token", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"capability_warnings":[]`) {
		t.Fatalf("empty warnings must be a JSON array: %s", encoded)
	}
	if result.BootstrapTLSMode != string(DownloadTLSStrict) || strings.Contains(result.Command, "--insecure") {
		t.Fatalf("strict download command relaxed TLS: %#v", result)
	}
}

func TestBuildPOSIXCreatesOneCommandAndStrictBootstrap(t *testing.T) {
	token := "secret-token-value"
	result, err := BuildPOSIX(POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxAMD64,
		InstallerURL: "https://artifacts.example.com/install.sh", InstallerSHA256: strings.Repeat("a", 64),
		BootstrapScriptURL: "https://argus.example.com/api/v1/connectors/bootstrap-script", DownloadTLSMode: DownloadTLSInsecureFirstFetch,
		TrustBundlePEM: testBundle(t), TrustBundleEpoch: 3, Token: token, ExpiresAt: time.Now().Add(time.Hour),
		InstallerArguments: []string{"--server", "https://argus.example.com", "--role", "bastion"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--cacert \"$ARGUS_CA_FILE\"", "sha256sum -c -", "'--token-file' \"$ARGUS_TOKEN_FILE\"", "'--scope' 'linux-system'"} {
		if !strings.Contains(result.BootstrapScript, expected) {
			t.Fatalf("bootstrap script does not contain %q", expected)
		}
	}
	if !strings.Contains(result.BootstrapScript, token) {
		t.Fatal("bootstrap script does not contain the token")
	}
	if !strings.Contains(result.Command, "--insecure") || !strings.Contains(result.Command, "X-Argus-Enrollment-Token: "+token) ||
		!strings.Contains(result.Command, "scope=linux-system") || strings.Contains(result.Command, "\n") ||
		strings.Contains(result.Command, "curl -fsSL") {
		t.Fatalf("insecure first-fetch command is not a single authenticated download: %q", result.Command)
	}
	if result.BootstrapTLSMode != string(DownloadTLSInsecureFirstFetch) {
		t.Fatalf("download TLS mode = %q", result.BootstrapTLSMode)
	}
	if strings.Contains(result.BootstrapScript, "--insecure") || strings.Contains(result.BootstrapScript, "curl -k") || strings.Contains(result.BootstrapScript, "| sudo") || strings.Contains(result.BootstrapScript, " | bash ") || strings.Contains(result.BootstrapScript, " | sh ") {
		t.Fatalf("unsafe bootstrap generated:\n%s", result.BootstrapScript)
	}
}

func TestGeneratedPOSIXCommandsParse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell is not required on Windows")
	}
	result, err := BuildPOSIX(POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxAMD64,
		InstallerURL: "https://artifacts.example.com/install.sh", InstallerSHA256: strings.Repeat("a", 64),
		BootstrapScriptURL: "https://argus.example.com/api/v1/connectors/bootstrap-script",
		DownloadTLSMode:    DownloadTLSStrict,
		TrustBundlePEM:     testBundle(t), TrustBundleEpoch: 1, Token: "token", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for name, command := range map[string]string{"command": result.Command, "bootstrap": result.BootstrapScript} {
		check := exec.Command("sh", "-n")
		check.Stdin = strings.NewReader(command)
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("%s command does not parse: %v: %s", name, err, output)
		}
	}
}

func TestBuildPOSIXCreatesOneKubernetesCommandWithoutTLSRelaxation(t *testing.T) {
	result, err := BuildPOSIX(POSIXOptions{Scope: ScopeKubernetes,
		InstallerURL: "https://artifacts.example.com/install.sh", InstallerSHA256: strings.Repeat("a", 64),
		TrustBundlePEM: testBundle(t), TrustBundleEpoch: 1, Token: "token", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Command == "" || strings.Contains(result.Command, "\n") || result.BootstrapTLSMode != "" {
		t.Fatalf("Kubernetes did not receive one safe command: %#v", result)
	}
	if strings.Contains(result.Command, "--insecure") || strings.Contains(result.Command, " | sh") {
		t.Fatalf("Kubernetes command relaxed TLS or piped into sh: %q", result.Command)
	}
}

func TestInsecureFirstFetchCommandDownloadsThenExecutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute the POSIX bootstrap suite in the Linux test container")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is required to execute the generated bootstrap command")
	}
	requestSeen := make(chan bool, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestSeen <- request.Header.Get("X-Argus-Enrollment-Token") == "one-time-token" &&
			request.URL.Query().Get("scope") == "linux-system"
		_, _ = writer.Write([]byte("printf 'bootstrap-executed'\n"))
	}))
	defer server.Close()

	bootstrap := []byte("printf 'bootstrap-executed'\n")
	digest := sha256.Sum256(bootstrap)
	command := bootstrapDownloadCommand(server.URL+"/bootstrap?scope=linux-system", "one-time-token", DownloadTLSInsecureFirstFetch, "", hex.EncodeToString(digest[:]))
	process := exec.Command("sh", "-c", command)
	output, err := process.CombinedOutput()
	if err != nil {
		t.Fatalf("execute generated one-line command: %v: %s", err, output)
	}
	if string(output) != "bootstrap-executed" || !<-requestSeen {
		t.Fatalf("generated command output/header mismatch: output=%q", output)
	}
}

func TestBuildPOSIXRejectsInvalidTrustAndHTTP(t *testing.T) {
	base := POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxAMD64, InstallerURL: "http://example.com/install.sh", BootstrapScriptURL: "https://example.com/bootstrap-script", InstallerSHA256: strings.Repeat("0", 64),
		TrustBundlePEM: testBundle(t), TrustBundleEpoch: 1, Token: "token", ExpiresAt: time.Now().Add(time.Hour)}
	if _, err := BuildPOSIX(base); err == nil {
		t.Fatal("HTTP installer was accepted")
	}
	base.InstallerURL = "https://example.com/install.sh"
	base.TrustBundlePEM = []byte("not pem")
	if _, err := BuildPOSIX(base); err == nil {
		t.Fatal("invalid Trust Bundle was accepted")
	}
}

func TestRelayDialAddressCoversEveryBootstrapDownload(t *testing.T) {
	options := POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxARM64,
		InstallerURL: "https://artifacts.example.com/install.sh", InstallerSHA256: strings.Repeat("a", 64),
		BootstrapScriptURL: "https://argus.example.com/api/v1/connectors/bootstrap-script", DownloadTLSMode: DownloadTLSInsecureFirstFetch,
		HTTPSDialAddress: "bastion.internal:8443", TrustBundlePEM: testBundle(t), TrustBundleEpoch: 2,
		Token: "token", ExpiresAt: time.Now().Add(time.Hour)}
	result, err := BuildPOSIX(options)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"command": result.Command, "bootstrap": result.BootstrapScript} {
		if !strings.Contains(value, "--connect-to '::bastion.internal:8443'") {
			t.Fatalf("%s omitted the fixed TLS relay dial address: %s", name, value)
		}
	}
	if !strings.Contains(result.BootstrapScript, "'--https-dial-address' 'bastion.internal:8443'") {
		t.Fatal("installer did not receive the fixed Artifact relay address")
	}
	options.HTTPSDialAddress = "bad address"
	if _, err = BuildPOSIX(options); err == nil {
		t.Fatal("invalid relay dial address was accepted")
	}
}

func TestWindowsInstructionUsesPowerShellAndFixedTLSRelay(t *testing.T) {
	result, err := BuildWindows(WindowsOptions{Platform: installation.WindowsAMD64,
		InstallerURL: "https://artifacts.example.com/install.ps1", InstallerSHA256: strings.Repeat("b", 64),
		BootstrapScriptURL: "https://argus.example.com/api/v1/connectors/bootstrap-script", DownloadTLSMode: DownloadTLSStrict,
		HTTPSDialAddress: "10.0.0.5:8443", TrustBundlePEM: testBundle(t), TrustBundleEpoch: 2,
		Token: "token", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Shell != "powershell" || !strings.Contains(result.Command, "--connect-to '::10.0.0.5:8443'") ||
		!strings.Contains(result.BootstrapScript, "'-HttpsDialAddress' '10.0.0.5:8443'") || strings.Contains(result.BootstrapScript, "--insecure") {
		t.Fatalf("Windows relay instruction is incomplete: %#v", result)
	}
}

func testBundle(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Argus Test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
}
