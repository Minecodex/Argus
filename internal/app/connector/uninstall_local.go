package connector

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/hostremoval"
)

type localUninstallOptions struct {
	OperationID       uuid.UUID
	ConnectorID       uuid.UUID
	RemovalGeneration int64
	JournalDirectory  string
	RelayHTTPSPort    int
	RelayGatewayPort  int
	RDPConfigStatus   string
}

type localCleanupResult struct {
	CollectorServiceAbsent bool
	CollectorProcessAbsent bool
	CollectorFilesAbsent   bool
	ConnectorServiceAbsent bool
	ConnectorProcessAbsent bool
	ConnectorFilesAbsent   bool
	ConnectorUserAbsent    bool
	RelayPortsReleased     bool
}

type localUninstallJournalEntry struct {
	Stage          string              `json:"stage"`
	Status         string              `json:"status"`
	Postconditions *localCleanupResult `json:"postconditions,omitempty"`
	ObservedAt     time.Time           `json:"observed_at"`
}

func runUninstallLocal(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("argus-connector uninstall-local", flag.ContinueOnError)
	operationValue := flags.String("operation-id", "", "Host removal operation ID")
	connectorValue := flags.String("connector-id", "", "Connector ID being removed")
	generation := flags.Int64("removal-generation", 0, "Removal fencing generation")
	journal := flags.String("journal-dir", "", "Uninstaller journal directory")
	relayHTTPS := flags.Int("relay-https-port", 0, "Bastion HTTPS relay port")
	relayGateway := flags.Int("relay-gateway-port", 0, "Bastion Gateway relay port")
	rdpStatus := flags.String("rdp-config-status", "not_applicable", "RDP restore result")
	if err := flags.Parse(args); err != nil {
		return err
	}
	operationID, operationErr := uuid.Parse(*operationValue)
	connectorID, connectorErr := uuid.Parse(*connectorValue)
	cleanJournal := filepath.Clean(*journal)
	if operationErr != nil || connectorErr != nil || *generation < 1 || !filepath.IsAbs(cleanJournal) ||
		filepath.Base(cleanJournal) != operationID.String() || *relayHTTPS < 0 || *relayHTTPS > 65535 || *relayGateway < 0 || *relayGateway > 65535 ||
		(*rdpStatus != "not_applicable" && *rdpStatus != "restored" && *rdpStatus != "drifted") {
		return errors.New("uninstall-local arguments are invalid")
	}
	if err := os.MkdirAll(cleanJournal, 0o700); err != nil {
		return err
	}
	if err := ensureLocalRemovalBinding(defaultDataDirectory(), cleanJournal, removalBinding{OperationID: operationID, ConnectorID: connectorID, Generation: *generation}); err != nil {
		return err
	}
	if err := writeLocalUninstallJournal(cleanJournal, "uninstalling_workloads", "running", nil); err != nil {
		return err
	}
	result := cleanupLocalPlatform(ctx, localUninstallOptions{OperationID: operationID, ConnectorID: connectorID,
		RemovalGeneration: *generation, JournalDirectory: cleanJournal, RelayHTTPSPort: *relayHTTPS,
		RelayGatewayPort: *relayGateway, RDPConfigStatus: *rdpStatus})
	if result.CollectorServiceAbsent && result.CollectorProcessAbsent && result.CollectorFilesAbsent {
		_ = writeLocalUninstallJournal(cleanJournal, "uninstalling_workloads", "succeeded", &result)
	} else {
		_ = writeLocalUninstallJournal(cleanJournal, "uninstalling_workloads", "failed", &result)
	}
	if *relayHTTPS != 0 || *relayGateway != 0 {
		status := "failed"
		if result.RelayPortsReleased {
			status = "succeeded"
		}
		_ = writeLocalUninstallJournal(cleanJournal, "stopping_relay", status, &result)
	}
	connectorStatus := "failed"
	if result.ConnectorServiceAbsent && result.ConnectorProcessAbsent && result.ConnectorFilesAbsent && result.ConnectorUserAbsent {
		connectorStatus = "succeeded"
	}
	_ = writeLocalUninstallJournal(cleanJournal, "uninstalling_connector", connectorStatus, &result)
	evidence := hostremoval.CleanupEvidence{SchemaVersion: hostremoval.CleanupEvidenceSchema, OperationID: operationID, ConnectorID: connectorID,
		RemovalGeneration: *generation, Platform: runtime.GOOS, CollectorServiceAbsent: result.CollectorServiceAbsent,
		CollectorProcessAbsent: result.CollectorProcessAbsent, CollectorFilesAbsent: result.CollectorFilesAbsent, ConnectorServiceAbsent: result.ConnectorServiceAbsent,
		ConnectorProcessAbsent: result.ConnectorProcessAbsent,
		ConnectorFilesAbsent:   result.ConnectorFilesAbsent, ConnectorUserAbsent: result.ConnectorUserAbsent,
		RelayPortsReleased: result.RelayPortsReleased, RDPConfigStatus: *rdpStatus, ObservedAt: time.Now().UTC()}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	verified := result.CollectorServiceAbsent && result.CollectorProcessAbsent && result.CollectorFilesAbsent && result.ConnectorServiceAbsent && result.ConnectorProcessAbsent && result.ConnectorFilesAbsent &&
		result.ConnectorUserAbsent && result.RelayPortsReleased
	evidenceName := "evidence.failed.json"
	if verified {
		evidenceName = "evidence.json"
	}
	temporary := filepath.Join(cleanJournal, evidenceName+".tmp")
	if err = os.WriteFile(temporary, append(encoded, '\n'), 0o600); err != nil {
		return err
	}
	if err = replaceUninstallJournalFile(temporary, filepath.Join(cleanJournal, evidenceName)); err != nil {
		return err
	}
	verificationStatus := "failed"
	if verified {
		verificationStatus = "succeeded"
	}
	_ = writeLocalUninstallJournal(cleanJournal, "verifying_cleanup", verificationStatus, &result)
	_, _ = os.Stdout.Write(append(encoded, '\n'))
	if !verified {
		return fmt.Errorf("local cleanup postconditions failed: collector_service=%s collector_process=%s collector_files=%s connector_service=%s connector_process=%s connector_files=%s connector_user=%s relay_ports=%s",
			strconv.FormatBool(result.CollectorServiceAbsent), strconv.FormatBool(result.CollectorProcessAbsent), strconv.FormatBool(result.CollectorFilesAbsent), strconv.FormatBool(result.ConnectorServiceAbsent),
			strconv.FormatBool(result.ConnectorProcessAbsent), strconv.FormatBool(result.ConnectorFilesAbsent), strconv.FormatBool(result.ConnectorUserAbsent), strconv.FormatBool(result.RelayPortsReleased))
	}
	return nil
}

func writeLocalUninstallJournal(directory, stage, status string, result *localCleanupResult) error {
	entry := localUninstallJournalEntry{Stage: stage, Status: status, Postconditions: result, ObservedAt: time.Now().UTC()}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	stateTemporary := filepath.Join(directory, "state.json.tmp")
	if err = os.WriteFile(stateTemporary, append(encoded, '\n'), 0o600); err != nil {
		return err
	}
	if err = replaceUninstallJournalFile(stateTemporary, filepath.Join(directory, "state.json")); err != nil {
		return err
	}
	events, err := os.OpenFile(filepath.Join(directory, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := events.Write(append(encoded, '\n'))
	closeErr := events.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
