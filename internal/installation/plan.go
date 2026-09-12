package installation

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type Artifact struct {
	Platform     string `json:"platform"`
	URI          string `json:"uri"`
	SHA256       string `json:"sha256"`
	Signature    string `json:"signature"`
	SigningKeyID string `json:"signing_key_id"`
	ByteSize     int64  `json:"byte_size"`
}

type HostConnectorInstallPlan struct {
	HostID              uuid.UUID     `json:"host_id"`
	ConnectorID         uuid.UUID     `json:"connector_id"`
	TargetPlatform      Platform      `json:"target_platform"`
	DistributionVersion string        `json:"distribution_version"`
	SSHPath             string        `json:"ssh_path"`
	ControlPath         string        `json:"control_path"`
	BastionScopeID      uuid.NullUUID `json:"bastion_scope_id,omitempty"`
	Address             string        `json:"address"`
	Port                int32         `json:"port"`
	Username            string        `json:"username"`
	CredentialID        uuid.UUID     `json:"credential_id"`
	CredentialVersion   int64         `json:"credential_version"`
	PinnedHostKey       string        `json:"pinned_host_key"`
	ReleaseVersionID    uuid.UUID     `json:"release_version_id"`
	ManifestURI         string        `json:"manifest_uri"`
	Artifact            Artifact      `json:"artifact"`
	SigningPublicKey    string        `json:"signing_public_key"`
	EnrollmentEndpoint  string        `json:"enrollment_endpoint"`
	GatewayEndpoint     string        `json:"gateway_endpoint"`
	EnrollDialAddress   string        `json:"enroll_dial_address,omitempty"`
	GatewayDialAddress  string        `json:"gateway_dial_address,omitempty"`
	TrustBundlePEM      []byte        `json:"trust_bundle_pem"`
	TrustBundleEpoch    int64         `json:"trust_bundle_epoch"`
	TrustBundleSHA256   string        `json:"trust_bundle_sha256"`
}

func (plan HostConnectorInstallPlan) Marshal() ([]byte, error) { return json.Marshal(plan) }

func (plan HostConnectorInstallPlan) CallbackAddresses() ([]string, error) {
	resolve := func(endpoint, override, scheme string) (string, error) {
		if override != "" {
			if host, port, err := net.SplitHostPort(override); err == nil && host != "" && port != "" {
				return override, nil
			}
			return "", errors.New("callback dial override must be host:port")
		}
		parsed, err := url.Parse(strings.TrimSpace(endpoint))
		if err != nil || parsed.Scheme != scheme || parsed.Hostname() == "" {
			return "", errors.New("callback endpoint is invalid")
		}
		port := parsed.Port()
		if port == "" {
			port = "443"
		}
		return net.JoinHostPort(parsed.Hostname(), port), nil
	}
	enrollment, err := resolve(plan.EnrollmentEndpoint, plan.EnrollDialAddress, "https")
	if err != nil {
		return nil, err
	}
	gateway, err := resolve(plan.GatewayEndpoint, plan.GatewayDialAddress, "grpcs")
	if err != nil {
		return nil, err
	}
	return []string{enrollment, gateway}, nil
}
