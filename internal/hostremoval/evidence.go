package hostremoval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

const CleanupEvidenceSchema = "argus.host_cleanup_evidence/v1"

// CleanupEvidence is emitted by the signed Connector binary after it has
// been copied outside the install root and invoked as the local uninstaller.
type CleanupEvidence struct {
	SchemaVersion          string    `json:"schema_version"`
	OperationID            uuid.UUID `json:"operation_id"`
	ConnectorID            uuid.UUID `json:"connector_id"`
	RemovalGeneration      int64     `json:"removal_generation"`
	Platform               string    `json:"platform"`
	CollectorServiceAbsent bool      `json:"collector_service_absent"`
	CollectorProcessAbsent bool      `json:"collector_process_absent"`
	CollectorFilesAbsent   bool      `json:"collector_files_absent"`
	ConnectorServiceAbsent bool      `json:"connector_service_absent"`
	ConnectorProcessAbsent bool      `json:"connector_process_absent"`
	ConnectorFilesAbsent   bool      `json:"connector_files_absent"`
	ConnectorUserAbsent    bool      `json:"connector_user_absent"`
	RelayPortsReleased     bool      `json:"relay_ports_released"`
	RDPConfigStatus        string    `json:"rdp_config_status"`
	ObservedAt             time.Time `json:"observed_at"`
}

func ParseCleanupEvidence(raw []byte, operationID, connectorID uuid.UUID, generation int64) (CleanupEvidence, []byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var evidence CleanupEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return CleanupEvidence{}, nil, errors.New("cleanup evidence is invalid")
	}
	if err := ensureJSONEOF(decoder); err != nil || evidence.SchemaVersion != CleanupEvidenceSchema || evidence.OperationID != operationID ||
		evidence.ConnectorID != connectorID || evidence.RemovalGeneration != generation || evidence.Platform != "linux" && evidence.Platform != "windows" ||
		!evidence.CollectorServiceAbsent || !evidence.CollectorProcessAbsent || !evidence.CollectorFilesAbsent || !evidence.ConnectorServiceAbsent || !evidence.ConnectorProcessAbsent || !evidence.ConnectorFilesAbsent ||
		!evidence.ConnectorUserAbsent || !evidence.RelayPortsReleased || evidence.ObservedAt.IsZero() ||
		evidence.RDPConfigStatus != "not_applicable" && evidence.RDPConfigStatus != "restored" && evidence.RDPConfigStatus != "drifted" {
		return CleanupEvidence{}, nil, errors.New("cleanup evidence postconditions are not satisfied")
	}
	canonical, err := json.Marshal(evidence)
	if err != nil {
		return CleanupEvidence{}, nil, err
	}
	return evidence, canonical, nil
}

func CleanupEvidenceDigest(canonical []byte) []byte {
	digest := sha256.Sum256(canonical)
	return digest[:]
}

func CleanupEvidenceDigestHex(canonical []byte) string {
	return hex.EncodeToString(CleanupEvidenceDigest(canonical))
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("cleanup evidence contains trailing data")
	}
	return nil
}
