// Package hostremoval owns the durable, fenced uninstall lifecycle for
// managed Hosts and Bastion root Hosts. Installation provenance chooses the
// delivery path; local cleanup and server-side identity revocation remain
// separate, observable steps.
package hostremoval

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTarget        = errors.New("host removal target is invalid")
	ErrDependenciesExist    = errors.New("HOST_REMOVAL_DEPENDENCIES_EXIST")
	ErrConnectionTestNeeded = errors.New("HOST_REMOVAL_CONNECTION_TEST_REQUIRED")
	ErrNotInstalled         = errors.New("HOST_REMOVAL_NOT_INSTALLED")
	ErrIdentityChanged      = errors.New("TARGET_IDENTITY_CHANGED")
	ErrOperationState       = errors.New("HOST_REMOVAL_STATE_CONFLICT")
	ErrTokenInvalid         = errors.New("HOST_REMOVAL_TOKEN_INVALID")
)

const (
	TargetManagedHost = "managed_host"
	TargetBastion     = "bastion_scope"
	ModeUninstall     = "uninstall"
	ModeForget        = "forget"
)

type PreviewInput struct {
	TargetType       string
	TargetID         uuid.UUID
	ExpectedVersion  int64
	Mode             string
	ConnectionTestID uuid.NullUUID
	CredentialID     uuid.NullUUID
	ConfirmationName string
}

type Dependency struct {
	Type   string    `json:"type"`
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Reason string    `json:"reason"`
}

type connectionSnapshot struct {
	TargetType        string        `json:"target_type"`
	Address           string        `json:"address"`
	Port              int32         `json:"port"`
	Platform          string        `json:"platform"`
	Username          string        `json:"username"`
	SSHPath           string        `json:"ssh_path"`
	BastionScopeID    uuid.NullUUID `json:"bastion_scope_id"`
	ConnectorID       uuid.NullUUID `json:"connector_id"`
	CredentialID      uuid.NullUUID `json:"credential_id"`
	CredentialVersion int64         `json:"credential_version"`
}

type connectionEvidence struct {
	HostKeyFingerprint string `json:"host_key_fingerprint"`
	Platform           string `json:"platform"`
	Architecture       string `json:"architecture"`
	Privileged         bool   `json:"privileged"`
}

// Plan is the immutable operation authority. No credential secret or token is
// stored here; only versions and identifiers cross the persistence boundary.
type Plan struct {
	SchemaVersion        string          `json:"schema_version"`
	OperationID          uuid.NullUUID   `json:"operation_id,omitempty"`
	TargetType           string          `json:"target_type"`
	TargetID             uuid.UUID       `json:"target_id"`
	HostID               uuid.UUID       `json:"host_id"`
	BastionScopeID       uuid.NullUUID   `json:"bastion_scope_id,omitempty"`
	Name                 string          `json:"name"`
	Mode                 string          `json:"mode"`
	ExpectedVersion      int64           `json:"expected_version"`
	ConnectorID          uuid.UUID       `json:"connector_id"`
	ConnectorVersion     int64           `json:"connector_version"`
	ConnectionEpoch      int64           `json:"connection_epoch"`
	RemovalGeneration    int64           `json:"removal_generation"`
	DeliveryMethod       string          `json:"delivery_method"`
	SSHPath              string          `json:"ssh_path"`
	TargetPlatform       string          `json:"target_platform"`
	ControlPath          string          `json:"control_path"`
	ConnectionTestID     uuid.NullUUID   `json:"connection_test_id,omitempty"`
	CredentialID         uuid.NullUUID   `json:"credential_id,omitempty"`
	CredentialVersion    int64           `json:"credential_version,omitempty"`
	Address              string          `json:"address,omitempty"`
	Port                 int32           `json:"port,omitempty"`
	Username             string          `json:"username,omitempty"`
	PinnedHostKey        string          `json:"pinned_host_key,omitempty"`
	HTTPSDialAddress     string          `json:"https_dial_address,omitempty"`
	RelayHTTPSPort       int32           `json:"relay_https_port,omitempty"`
	RelayGatewayPort     int32           `json:"relay_gateway_port,omitempty"`
	TrustBundleEpoch     int64           `json:"trust_bundle_epoch"`
	Dependencies         []Dependency    `json:"dependencies"`
	ComponentInventory   json.RawMessage `json:"component_inventory"`
	ManagedChangeID      uuid.NullUUID   `json:"managed_change_id,omitempty"`
	ManagedChangeBefore  json.RawMessage `json:"managed_change_before,omitempty"`
	ManagedChangeApplied json.RawMessage `json:"managed_change_applied,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
}

type impactSnapshot struct {
	TargetType        string       `json:"target_type"`
	TargetID          uuid.UUID    `json:"target_id"`
	ResourceVersion   int64        `json:"resource_version"`
	ConnectorID       uuid.UUID    `json:"connector_id"`
	ConnectorVersion  int64        `json:"connector_version"`
	ConnectionEpoch   int64        `json:"connection_epoch"`
	RemovalGeneration int64        `json:"removal_generation"`
	Dependencies      []Dependency `json:"dependencies"`
}

type Receipt struct {
	OperationID       uuid.UUID       `json:"operation_id"`
	RemovalGeneration int64           `json:"removal_generation"`
	ConnectorID       uuid.UUID       `json:"connector_id"`
	Stage             string          `json:"stage"`
	LocalCleanup      string          `json:"local_cleanup"`
	ResultHash        string          `json:"result_hash"`
	ErrorCode         string          `json:"error_code,omitempty"`
	Evidence          json.RawMessage `json:"evidence"`
}

type OperationView struct {
	Operation any
	Events    any
}
