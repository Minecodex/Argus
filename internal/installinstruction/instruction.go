// Package installinstruction builds self-contained, fail-closed installation
// instructions. The generated bootstrap never changes the operating-system
// trust store and never executes bytes received directly from a pipe.
package installinstruction

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/installation"
)

type Scope string

type DownloadTLSMode string

const (
	ScopeLinuxSystem   Scope = "linux-system"
	ScopeKubernetes    Scope = "kubernetes"
	ScopeWindowsSystem Scope = "windows-system"

	DownloadTLSStrict             DownloadTLSMode = "strict"
	DownloadTLSInsecureFirstFetch DownloadTLSMode = "insecure-first-fetch"
)

type Set struct {
	// Scope is persisted only inside the encrypted one-time result envelope.
	// Public APIs project the platform-specific fields explicitly.
	Scope              Scope     `json:"scope"`
	Platform           string    `json:"platform"`
	Shell              string    `json:"shell"`
	Privilege          string    `json:"privilege"`
	Command            string    `json:"command"`
	BootstrapTLSMode   string    `json:"bootstrap_tls_mode,omitempty"`
	ReleaseVersion     string    `json:"release_version"`
	ExpiresAt          time.Time `json:"expires_at"`
	TrustBundleEpoch   int64     `json:"trust_bundle_epoch"`
	TrustBundleSHA256  string    `json:"trust_bundle_sha256"`
	BootstrapSHA256    string    `json:"bootstrap_sha256"`
	InstallerSHA256    string    `json:"installer_sha256"`
	CapabilityWarnings []string  `json:"capability_warnings"`

	// BootstrapScript is generated only while serving an authenticated dynamic
	// bootstrap request. It is never serialized into one-time result payloads.
	BootstrapScript string `json:"-"`
}

type POSIXOptions struct {
	Scope              Scope
	Platform           installation.Platform
	InstallerURL       string
	BootstrapScriptURL string
	HTTPSDialAddress   string
	DownloadTLSMode    DownloadTLSMode
	InstallerSHA256    string
	TrustBundlePEM     []byte
	TrustBundleEpoch   int64
	Token              string
	ExpiresAt          time.Time
	InstallerArguments []string
	CapabilityWarnings []string
	ReleaseVersion     string
}

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// BuildPOSIX creates the single user-facing install command and the strict
// bootstrap script served behind it. Arguments must not contain the enrollment
// token; the script passes it through a mode-0600 temporary file.
func BuildPOSIX(options POSIXOptions) (Set, error) {
	if options.Scope != ScopeLinuxSystem && options.Scope != ScopeKubernetes {
		return Set{}, errors.New("installation scope is invalid")
	}
	if options.Scope == ScopeLinuxSystem && options.Platform != installation.LinuxAMD64 && options.Platform != installation.LinuxARM64 {
		return Set{}, errors.New("Linux installation platform is invalid")
	}
	parsed, err := url.Parse(strings.TrimSpace(options.InstallerURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return Set{}, errors.New("installer URL must be an absolute HTTPS URL without user info")
	}
	var command, dynamicBootstrapURL string
	downloadTLSMode := options.DownloadTLSMode
	if strings.TrimSpace(options.BootstrapScriptURL) != "" {
		bootstrapURL, bootstrapErr := url.Parse(strings.TrimSpace(options.BootstrapScriptURL))
		if bootstrapErr != nil || bootstrapURL.Scheme != "https" || bootstrapURL.Host == "" || bootstrapURL.User != nil {
			return Set{}, errors.New("bootstrap script URL must be an absolute HTTPS URL without user info")
		}
		if err := downloadTLSMode.Validate(); err != nil {
			return Set{}, err
		}
		query := bootstrapURL.Query()
		query.Set("scope", string(options.Scope))
		bootstrapURL.RawQuery = query.Encode()
		if err := validateDialAddress(options.HTTPSDialAddress); err != nil {
			return Set{}, err
		}
		dynamicBootstrapURL = bootstrapURL.String()
	} else if downloadTLSMode != "" {
		return Set{}, errors.New("bootstrap download TLS mode requires a bootstrap script URL")
	}
	digest := strings.ToLower(strings.TrimSpace(options.InstallerSHA256))
	if !sha256Pattern.MatchString(digest) {
		return Set{}, errors.New("installer SHA-256 is invalid")
	}
	canonical, err := canonicalCABundle(options.TrustBundlePEM)
	if err != nil {
		return Set{}, err
	}
	if options.TrustBundleEpoch < 1 || strings.TrimSpace(options.Token) == "" || options.ExpiresAt.IsZero() {
		return Set{}, errors.New("installation token, expiration, and positive Trust Bundle epoch are required")
	}
	for _, argument := range options.InstallerArguments {
		if strings.ContainsRune(argument, '\x00') {
			return Set{}, errors.New("installer argument contains NUL")
		}
	}
	bundleDigest := sha256.Sum256(canonical)
	bundleBase64 := base64.StdEncoding.EncodeToString(canonical)
	base := bootstrapPrefix(parsed.String(), digest, bundleBase64, options.HTTPSDialAddress)
	args := append([]string{"--scope", string(options.Scope), "--ca-file", `"$ARGUS_CA_FILE"`, "--token-file", `"$ARGUS_TOKEN_FILE"`}, options.InstallerArguments...)
	if options.HTTPSDialAddress != "" {
		args = append(args, "--https-dial-address", options.HTTPSDialAddress)
	}
	commandSuffix := "sh \"$ARGUS_INSTALLER\" " + shellArguments(args)
	bootstrapScript := base + "\nprintf '%s' " + quote(options.Token) + " > \"$ARGUS_TOKEN_FILE\"\n" + commandSuffix
	bootstrapHash := sha256.Sum256([]byte(bootstrapScript + "\n"))
	bootstrapDigest := hex.EncodeToString(bootstrapHash[:])
	if dynamicBootstrapURL != "" {
		command = bootstrapDownloadCommand(dynamicBootstrapURL, options.Token, downloadTLSMode, options.HTTPSDialAddress, bootstrapDigest)
	} else {
		command = inlineScriptCommand(bootstrapScript)
	}
	platform := string(options.Platform)
	if options.Scope == ScopeKubernetes {
		platform = "kubernetes"
	}
	return Set{
		Scope: options.Scope, Platform: platform, Shell: string(installation.POSIXShell), Privilege: "system",
		Command: command, BootstrapTLSMode: string(downloadTLSMode), ReleaseVersion: options.ReleaseVersion,
		ExpiresAt: options.ExpiresAt.UTC(), TrustBundleEpoch: options.TrustBundleEpoch,
		TrustBundleSHA256: hex.EncodeToString(bundleDigest[:]), BootstrapSHA256: bootstrapDigest, InstallerSHA256: digest,
		CapabilityWarnings: append([]string{}, options.CapabilityWarnings...),
		BootstrapScript:    bootstrapScript,
	}, nil
}

type WindowsOptions struct {
	Platform           installation.Platform
	InstallerURL       string
	BootstrapScriptURL string
	HTTPSDialAddress   string
	DownloadTLSMode    DownloadTLSMode
	InstallerSHA256    string
	TrustBundlePEM     []byte
	TrustBundleEpoch   int64
	Token              string
	ExpiresAt          time.Time
	InstallerArguments []string
	CapabilityWarnings []string
	ReleaseVersion     string
}

func BuildWindows(options WindowsOptions) (Set, error) {
	if options.Platform != installation.WindowsAMD64 {
		return Set{}, errors.New("Windows installation platform is invalid")
	}
	installerURL, err := absoluteHTTPSURL(options.InstallerURL, "installer")
	if err != nil {
		return Set{}, err
	}
	bootstrapURL, err := absoluteHTTPSURL(options.BootstrapScriptURL, "bootstrap script")
	if err != nil {
		return Set{}, err
	}
	if err = options.DownloadTLSMode.Validate(); err != nil {
		return Set{}, err
	}
	if err = validateDialAddress(options.HTTPSDialAddress); err != nil {
		return Set{}, err
	}
	digest := strings.ToLower(strings.TrimSpace(options.InstallerSHA256))
	if !sha256Pattern.MatchString(digest) {
		return Set{}, errors.New("installer SHA-256 is invalid")
	}
	canonical, err := canonicalCABundle(options.TrustBundlePEM)
	if err != nil {
		return Set{}, err
	}
	if options.TrustBundleEpoch < 1 || strings.TrimSpace(options.Token) == "" || options.ExpiresAt.IsZero() {
		return Set{}, errors.New("installation token, expiration, and positive Trust Bundle epoch are required")
	}
	query := bootstrapURL.Query()
	query.Set("scope", string(ScopeWindowsSystem))
	bootstrapURL.RawQuery = query.Encode()
	bundleDigest := sha256.Sum256(canonical)
	bundleBase64 := base64.StdEncoding.EncodeToString(canonical)
	arguments := append([]string(nil), options.InstallerArguments...)
	if options.HTTPSDialAddress != "" {
		arguments = append(arguments, "-HttpsDialAddress", options.HTTPSDialAddress)
	}
	bootstrap := windowsBootstrapScript(installerURL.String(), digest, bundleBase64, options.Token, arguments, options.HTTPSDialAddress)
	bootstrapHash := sha256.Sum256([]byte(bootstrap + "\n"))
	bootstrapDigest := hex.EncodeToString(bootstrapHash[:])
	command := windowsBootstrapDownloadCommand(bootstrapURL.String(), options.Token, options.DownloadTLSMode, options.HTTPSDialAddress, bootstrapDigest)
	return Set{Scope: ScopeWindowsSystem, Platform: string(options.Platform), Shell: string(installation.PowerShell), Privilege: "system",
		Command: command, BootstrapTLSMode: string(options.DownloadTLSMode), ReleaseVersion: options.ReleaseVersion,
		ExpiresAt: options.ExpiresAt.UTC(), TrustBundleEpoch: options.TrustBundleEpoch,
		TrustBundleSHA256: hex.EncodeToString(bundleDigest[:]), BootstrapSHA256: bootstrapDigest, InstallerSHA256: digest,
		CapabilityWarnings: append([]string{}, options.CapabilityWarnings...), BootstrapScript: bootstrap}, nil
}

func absoluteHTTPSURL(value, purpose string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("%s URL must be an absolute HTTPS URL without user info", purpose)
	}
	return parsed, nil
}

func windowsBootstrapDownloadCommand(scriptURL, token string, mode DownloadTLSMode, dialAddress, bootstrapSHA256 string) string {
	tlsFlag := ""
	if mode == DownloadTLSInsecureFirstFetch {
		tlsFlag = " --insecure" // ARGUS_INSECURE_FIRST_FETCH_ONLY
	}
	connect := curlConnectToPowerShell(dialAddress)
	return `$ErrorActionPreference='Stop'; $d=Join-Path ([IO.Path]::GetTempPath()) ('argus-bootstrap-'+[Guid]::NewGuid().ToString('N')); New-Item -ItemType Directory -Path $d | Out-Null; try { $s=Join-Path $d 'bootstrap.ps1'; & curl.exe -fsS --proto '=https' --tlsv1.2` + tlsFlag + connect + ` --header ` + powerShellQuote("X-Argus-Enrollment-Token: "+token) + ` --output $s ` + powerShellQuote(scriptURL) + `; if ($LASTEXITCODE -ne 0) { throw 'Argus bootstrap download failed' }; if ((Get-FileHash -LiteralPath $s -Algorithm SHA256).Hash.ToLowerInvariant() -ne ` + powerShellQuote(bootstrapSHA256) + `) { throw 'Argus bootstrap SHA-256 mismatch' }; & powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $s } finally { Remove-Item -LiteralPath $d -Recurse -Force -ErrorAction SilentlyContinue }`
}

func windowsBootstrapScript(installerURL, digest, bundleBase64, token string, arguments []string, dialAddress string) string {
	quoted := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		quoted = append(quoted, powerShellQuote(argument))
	}
	return `$ErrorActionPreference="Stop"
$ProgressPreference="SilentlyContinue"
Set-StrictMode -Version Latest
$d=Join-Path ([IO.Path]::GetTempPath()) ("argus-install-"+[Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $d | Out-Null
try {
  $ca=Join-Path $d "argus-ca.pem"
  $installer=Join-Path $d "install.ps1"
  $tokenFile=Join-Path $d "token"
  [IO.File]::WriteAllBytes($ca,[Convert]::FromBase64String(` + powerShellQuote(bundleBase64) + `))
  & curl.exe -fsSL --proto '=https' --tlsv1.2 --ssl-revoke-best-effort --cacert $ca` + curlConnectToPowerShell(dialAddress) + ` --output $installer ` + powerShellQuote(installerURL) + `
  if ($LASTEXITCODE -ne 0) { throw "Argus installer download failed" }
  if ((Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant() -ne ` + powerShellQuote(digest) + `) { throw "Argus installer SHA-256 mismatch" }
  [IO.File]::WriteAllText($tokenFile,` + powerShellQuote(token) + `,(New-Object Text.UTF8Encoding($false)))
  & powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $installer ` + strings.Join(quoted, " ") + ` -CAFile $ca -TokenFile $tokenFile
  if ($LASTEXITCODE -ne 0) { throw "Argus installer failed" }
} finally { Remove-Item -LiteralPath $d -Recurse -Force -ErrorAction SilentlyContinue }
`
}

func powerShellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// bootstrapDownloadCommand is the short, user-facing entry point. In
// insecure-first-fetch mode only this request skips server certificate
// verification; the downloaded script contains the versioned Trust Bundle and
// performs every subsequent request in strict mode.
func bootstrapDownloadCommand(scriptURL, token string, mode DownloadTLSMode, dialAddress, bootstrapSHA256 string) string {
	curlTLS := ""
	if mode == DownloadTLSInsecureFirstFetch {
		curlTLS = " --insecure" // ARGUS_INSECURE_FIRST_FETCH_ONLY
	}
	return `(set -eu; umask 077; ARGUS_BOOTSTRAP=$(mktemp "${TMPDIR:-/tmp}/argus-bootstrap.XXXXXX"); trap 'rm -f "$ARGUS_BOOTSTRAP"' EXIT HUP INT TERM; curl -fsS --proto '=https' --tlsv1.2` + curlTLS + curlConnectToShell(dialAddress) + ` --header ` + quote("X-Argus-Enrollment-Token: "+token) + ` --output "$ARGUS_BOOTSTRAP" ` + quote(scriptURL) + `; printf '%s  %s\n' ` + quote(bootstrapSHA256) + ` "$ARGUS_BOOTSTRAP" | sha256sum -c - >/dev/null; chmod 0700 "$ARGUS_BOOTSTRAP"; sh "$ARGUS_BOOTSTRAP")`
}

// inlineScriptCommand keeps Kubernetes enrollment on the same single-command
// contract when no authenticated bootstrap endpoint is available. The script
// is written to a mode-0700 temporary file before execution, never piped to sh.
func inlineScriptCommand(script string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(script + "\n"))
	return `(set -eu; umask 077; ARGUS_BOOTSTRAP=$(mktemp "${TMPDIR:-/tmp}/argus-bootstrap.XXXXXX"); trap 'rm -f "$ARGUS_BOOTSTRAP"' EXIT HUP INT TERM; printf '%s' ` + quote(encoded) + ` | base64 -d > "$ARGUS_BOOTSTRAP"; chmod 0700 "$ARGUS_BOOTSTRAP"; sh "$ARGUS_BOOTSTRAP")`
}

func bootstrapPrefix(installerURL, installerSHA256, bundleBase64, dialAddress string) string {
	return `set -eu
umask 077
ARGUS_INSTALL_DIR=$(mktemp -d "${TMPDIR:-/tmp}/argus-install.XXXXXX")
ARGUS_CA_FILE="$ARGUS_INSTALL_DIR/argus-ca.pem"
ARGUS_INSTALLER="$ARGUS_INSTALL_DIR/install.sh"
ARGUS_TOKEN_FILE="$ARGUS_INSTALL_DIR/token"
argus_cleanup() { rm -rf "$ARGUS_INSTALL_DIR"; }
trap argus_cleanup EXIT HUP INT TERM
printf '%s' ` + quote(bundleBase64) + ` | base64 -d > "$ARGUS_CA_FILE"
curl -fsSL --proto '=https' --tlsv1.2 --cacert "$ARGUS_CA_FILE"` + curlConnectToShell(dialAddress) + ` --output "$ARGUS_INSTALLER" ` + quote(installerURL) + `
printf '%s  %s\n' ` + quote(installerSHA256) + ` "$ARGUS_INSTALLER" | sha256sum -c - >/dev/null
chmod 0700 "$ARGUS_INSTALLER"`
}

func validateDialAddress(value string) error {
	if value == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return errors.New("HTTPS dial address must be host:port")
	}
	return nil
}

func curlConnectToShell(value string) string {
	if value == "" {
		return ""
	}
	return " --connect-to " + quote("::"+value)
}

func curlConnectToPowerShell(value string) string {
	if value == "" {
		return ""
	}
	return " --connect-to " + powerShellQuote("::"+value)
}

func shellArguments(arguments []string) string {
	values := make([]string, 0, len(arguments))
	for _, value := range arguments {
		if strings.HasPrefix(value, `"$ARGUS_`) && strings.HasSuffix(value, `"`) {
			values = append(values, value)
		} else {
			values = append(values, quote(value))
		}
	}
	return strings.Join(values, " ")
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func canonicalCABundle(value []byte) ([]byte, error) {
	rest := bytes.TrimSpace(value)
	seen := map[[32]byte]struct{}{}
	var output bytes.Buffer
	count := 0
	now := time.Now().UTC()
	for len(rest) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("Trust Bundle must contain only PEM certificates")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !certificate.IsCA || !certificate.BasicConstraintsValid || certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
			return nil, errors.New("Trust Bundle contains a certificate that is not a valid CA")
		}
		if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			return nil, errors.New("Trust Bundle contains an expired or not-yet-valid CA")
		}
		digest := sha256.Sum256(block.Bytes)
		if _, exists := seen[digest]; exists {
			return nil, fmt.Errorf("Trust Bundle contains duplicate CA %s", hex.EncodeToString(digest[:]))
		}
		seen[digest] = struct{}{}
		_ = pem.Encode(&output, &pem.Block{Type: "CERTIFICATE", Bytes: block.Bytes})
		count++
		rest = bytes.TrimSpace(remaining)
	}
	if count == 0 {
		return nil, errors.New("Trust Bundle contains no CA certificates")
	}
	return output.Bytes(), nil
}
