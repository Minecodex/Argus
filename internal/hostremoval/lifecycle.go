package hostremoval

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var orderedStages = []string{
	"draining", "terminating_sessions", "uninstalling_workloads", "stopping_relay",
	"uninstalling_connector", "verifying_cleanup", "revoking_identities", "completed",
}

func (service Service) BeginRemote(ctx context.Context, operation db.HostRemovalOperation) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		current, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
		if err != nil || current.Status != "running" || current.RemovalGeneration != operation.RemovalGeneration || current.Attempts != operation.Attempts || current.LeaseOwner != operation.LeaseOwner {
			return ErrOperationState
		}
		if err = service.validateOperationFence(ctx, q, current); err != nil {
			return err
		}
		_, _ = q.TerminateRemoteAccessSessionsByHostRemoval(ctx, db.TerminateRemoteAccessSessionsByHostRemovalParams{EnterpriseID: current.EnterpriseID, HostID: current.HostID})
		_ = service.step(ctx, q, current, "terminating_sessions", "succeeded", map[string]any{"new_sessions_blocked": true}, "")
		_ = service.recordEvent(ctx, q, current, "terminating_sessions", "succeeded", "")
		if current.TargetType == TargetBastion {
			if rows, err := q.MarkRemovalBastionUninstalling(ctx, db.MarkRemovalBastionUninstallingParams{ID: current.BastionScopeID.UUID,
				EnterpriseID: current.EnterpriseID, ActiveConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true},
				RemovalGeneration: current.RemovalGeneration}); err != nil || rows != 1 {
				return ErrIdentityChanged
			}
		} else if rows, err := q.MarkRemovalHostUninstalling(ctx, db.MarkRemovalHostUninstallingParams{ID: current.HostID,
			EnterpriseID: current.EnterpriseID, ConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true},
			RemovalGeneration: current.RemovalGeneration}); err != nil || rows != 1 {
			return ErrIdentityChanged
		}
		_, err = q.AdvanceHostRemovalOperation(ctx, db.AdvanceHostRemovalOperationParams{ID: current.ID, EnterpriseID: current.EnterpriseID,
			Status: "running", Stage: "uninstalling_workloads"})
		if err == nil {
			_ = service.recordEvent(ctx, q, current, "uninstalling_workloads", "started", "")
		}
		return err
	})
}

func (service Service) SubmitReceipt(ctx context.Context, plainToken string, receipt Receipt) (View, error) {
	var view View
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		token, err := q.GetActiveHostRemovalToken(ctx, tokenDigest(plainToken))
		if err != nil || token.Purpose != "receipt" || token.OperationID != receipt.OperationID || token.KeyVersion != tokenKeyVersion {
			return ErrTokenInvalid
		}
		envelope, err := openToken(service.TokenKey, token.Nonce, token.Ciphertext, token.EnterpriseID, token.OperationID)
		if err != nil || subtle.ConstantTimeCompare([]byte(envelope.Token), []byte(plainToken)) != 1 {
			return ErrTokenInvalid
		}
		if token.Status == "consumed" {
			current, currentErr := q.GetHostRemovalOperation(ctx, db.GetHostRemovalOperationParams{ID: token.OperationID, EnterpriseID: token.EnterpriseID})
			if currentErr != nil || current.Status != "succeeded" {
				return ErrTokenInvalid
			}
			view, currentErr = service.viewWithQueries(ctx, q, token.EnterpriseID, token.OperationID)
			return currentErr
		}
		operation, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: receipt.OperationID, EnterpriseID: token.EnterpriseID})
		if err != nil || operation.RemovalGeneration != receipt.RemovalGeneration || operation.ConnectorID != receipt.ConnectorID || receipt.Stage != "verifying_cleanup" {
			return ErrIdentityChanged
		}
		if err = service.validateOperationFence(ctx, q, operation); err != nil {
			return err
		}
		evidence, canonical, evidenceErr := ParseCleanupEvidence(receipt.Evidence, operation.ID, operation.ConnectorID, operation.RemovalGeneration)
		expected := CleanupEvidenceDigestHex(canonical)
		if evidenceErr != nil || !validEvidenceForOperation(evidence, operation) || receipt.LocalCleanup != "verified" || subtle.ConstantTimeCompare([]byte(expected), []byte(receipt.ResultHash)) != 1 {
			return ErrTokenInvalid
		}
		rows, err := q.ConsumeHostRemovalToken(ctx, db.ConsumeHostRemovalTokenParams{ID: token.ID, OperationID: operation.ID})
		if err != nil || rows != 1 {
			return ErrTokenInvalid
		}
		if err = service.finalizeVerified(ctx, q, operation, evidence, canonical); err != nil {
			return err
		}
		view, err = service.viewWithQueries(ctx, q, operation.EnterpriseID, operation.ID)
		return err
	})
	return view, err
}

func (service Service) FinalizeTrusted(ctx context.Context, operation db.HostRemovalOperation, evidenceRaw, cleanupHash []byte) error {
	evidence, canonical, err := ParseCleanupEvidence(evidenceRaw, operation.ID, operation.ConnectorID, operation.RemovalGeneration)
	if err != nil || !validEvidenceForOperation(evidence, operation) || subtle.ConstantTimeCompare(CleanupEvidenceDigest(canonical), cleanupHash) != 1 {
		return ErrTokenInvalid
	}
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		current, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
		if err != nil || current.RemovalGeneration != operation.RemovalGeneration || current.ConnectorID != operation.ConnectorID || current.Attempts != operation.Attempts || current.LeaseOwner != operation.LeaseOwner {
			return ErrIdentityChanged
		}
		if err = service.validateOperationFence(ctx, q, current); err != nil {
			return err
		}
		return service.finalizeVerified(ctx, q, current, evidence, canonical)
	})
}

func (service Service) FinalizeTrustedWithQueries(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation, evidenceRaw, cleanupHash []byte) error {
	evidence, canonical, err := ParseCleanupEvidence(evidenceRaw, operation.ID, operation.ConnectorID, operation.RemovalGeneration)
	if err != nil || !validEvidenceForOperation(evidence, operation) || subtle.ConstantTimeCompare(CleanupEvidenceDigest(canonical), cleanupHash) != 1 {
		return ErrTokenInvalid
	}
	if err = service.validateOperationFence(ctx, q, operation); err != nil {
		return err
	}
	return service.finalizeVerified(ctx, q, operation, evidence, canonical)
}

func validEvidenceForOperation(evidence CleanupEvidence, operation db.HostRemovalOperation) bool {
	platform := strings.SplitN(operation.TargetPlatform, "_", 2)[0]
	return operation.CreatedAt.Valid && evidence.Platform == platform && !evidence.ObservedAt.Before(operation.CreatedAt.Time.Add(-5*time.Minute)) &&
		!evidence.ObservedAt.After(time.Now().UTC().Add(5*time.Minute))
}

func (service Service) finalizeVerified(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation, evidence CleanupEvidence, canonicalEvidence []byte) error {
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
	if err != nil || !removalConnectorMatches(operation, connector) {
		return ErrIdentityChanged
	}
	for _, stage := range []string{"uninstalling_workloads", "stopping_relay", "uninstalling_connector"} {
		if stage == "stopping_relay" && operation.TargetType != TargetBastion {
			continue
		}
		if err := service.step(ctx, q, operation, stage, "succeeded", map[string]any{"verified": true}, ""); err != nil {
			return err
		}
		_ = service.recordEvent(ctx, q, operation, stage, "succeeded", "")
	}
	if err := service.step(ctx, q, operation, "verifying_cleanup", "succeeded", map[string]any{"local_cleanup": "verified", "evidence": json.RawMessage(canonicalEvidence)}, ""); err != nil {
		return err
	}
	if _, err := q.AdvanceHostRemovalOperation(ctx, db.AdvanceHostRemovalOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID,
		Status: "running", Stage: "revoking_identities"}); err != nil {
		return err
	}
	if err := service.recordEvent(ctx, q, operation, "revoking_identities", "started", ""); err != nil {
		return err
	}
	_, _ = q.TerminateRemoteAccessSessionsByHostRemoval(ctx, db.TerminateRemoteAccessSessionsByHostRemovalParams{EnterpriseID: operation.EnterpriseID, HostID: operation.HostID})
	_, _ = q.InvalidateHostTelemetryForRemoval(ctx, db.InvalidateHostTelemetryForRemovalParams{EnterpriseID: operation.EnterpriseID, ResourceID: operation.HostID})
	_, _ = q.RevokeRemovalConnectorCommands(ctx, db.RevokeRemovalConnectorCommandsParams{EnterpriseID: operation.EnterpriseID, ConnectorID: operation.ConnectorID})
	_, _ = q.RevokeConnectorControlTunnelLeases(ctx, db.RevokeConnectorControlTunnelLeasesParams{ConnectorID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
	_, _ = q.MarkConnectorControlTunnelRemoved(ctx, db.MarkConnectorControlTunnelRemovedParams{ConnectorID: operation.ConnectorID,
		EnterpriseID: operation.EnterpriseID, LastDropReason: "host_removed"})
	if rows, err := q.FinalizeHostConnectorRemoval(ctx, db.FinalizeHostConnectorRemovalParams{ID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID,
		Version: connector.Version}); err != nil || rows != 1 {
		return ErrIdentityChanged
	}
	if err := q.RevokeConnectorCertificates(ctx, db.RevokeConnectorCertificatesParams{ConnectorID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID}); err != nil {
		return err
	}
	if err := q.RevokePKISubjectCertificates(ctx, db.RevokePKISubjectCertificatesParams{SubjectKind: "connector", SubjectID: operation.ConnectorID.String(),
		RevocationReason: "host_removed"}); err != nil {
		return err
	}
	journalStatus := "restored"
	operationCode := pgtype.Text{}
	if evidence.RDPConfigStatus == "drifted" {
		journalStatus = "drifted"
		operationCode = pgtype.Text{String: "LOCAL_CONFIG_DRIFT", Valid: true}
	}
	_, _ = q.CompleteHostManagedChangeJournal(ctx, db.CompleteHostManagedChangeJournalParams{EnterpriseID: operation.EnterpriseID,
		HostID: operation.HostID, ChangeType: "windows_rdp", Status: journalStatus})
	if operation.TargetType == TargetBastion {
		if rows, err := q.MarkRemovalBastionHostTerminal(ctx, db.MarkRemovalBastionHostTerminalParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID,
			ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
			Status: "uninstalled", LocalCleanup: "verified"}); err != nil || rows != 1 {
			return ErrIdentityChanged
		}
		if rows, err := q.MarkRemovalBastionTerminal(ctx, db.MarkRemovalBastionTerminalParams{ID: operation.BastionScopeID.UUID, EnterpriseID: operation.EnterpriseID,
			ActiveConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
			Status: "uninstalled", LocalCleanup: "verified"}); err != nil || rows != 1 {
			return ErrIdentityChanged
		}
	} else if rows, err := q.MarkRemovalHostTerminal(ctx, db.MarkRemovalHostTerminalParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID,
		ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
		Status: "uninstalled", LocalCleanup: "verified"}); err != nil || rows != 1 {
		return ErrIdentityChanged
	}
	if _, err := q.CompleteHostRemovalOperation(ctx, db.CompleteHostRemovalOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID,
		LocalCleanup: "verified", ErrorCode: operationCode}); err != nil {
		return err
	}
	if _, err := q.SupersedeHostRemovalCommands(ctx, db.SupersedeHostRemovalCommandsParams{EnterpriseID: operation.EnterpriseID, OperationRef: operation.ID.String()}); err != nil {
		return err
	}
	if err := service.step(ctx, q, operation, "revoking_identities", "succeeded", map[string]any{"identities_revoked": true}, ""); err != nil {
		return err
	}
	_ = service.recordEvent(ctx, q, operation, "completed", "succeeded", "")
	_, _ = q.RevokeHostRemovalTokens(ctx, db.RevokeHostRemovalTokensParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID})
	return appendRemovalAudit(ctx, q, operation, "host.removal.completed", "success", "")
}

func (service Service) markUnknown(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation, code string) error {
	_, err := q.FailHostRemovalOperation(ctx, db.FailHostRemovalOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID,
		Status: "cleanup_unknown", ErrorCode: pgtype.Text{String: code, Valid: true}, LocalCleanup: "unknown"})
	if err != nil {
		return err
	}
	if operation.TargetType == TargetBastion {
		_, _ = q.MarkRemovalBastionHostTerminal(ctx, db.MarkRemovalBastionHostTerminalParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID,
			ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
			Status: "cleanup_unknown", LocalCleanup: "unknown"})
		_, _ = q.MarkRemovalBastionTerminal(ctx, db.MarkRemovalBastionTerminalParams{ID: operation.BastionScopeID.UUID, EnterpriseID: operation.EnterpriseID,
			ActiveConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
			Status: "cleanup_unknown", LocalCleanup: "unknown"})
	} else {
		_, _ = q.MarkRemovalHostTerminal(ctx, db.MarkRemovalHostTerminalParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID,
			ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
			Status: "cleanup_unknown", LocalCleanup: "unknown"})
	}
	_ = service.step(ctx, q, operation, operation.Stage, "unknown", map[string]any{"local_cleanup": "unknown"}, code)
	if err := service.recordEvent(ctx, q, operation, operation.Stage, "unknown", code); err != nil {
		return err
	}
	return appendRemovalAudit(ctx, q, operation, "host.removal.cleanup_unknown", "failure", code)
}

func (service Service) Fail(ctx context.Context, operation db.HostRemovalOperation, code string, cleanupUnknown bool) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		current, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
		if err != nil {
			return err
		}
		if current.Attempts != operation.Attempts || current.LeaseOwner != operation.LeaseOwner {
			return ErrOperationState
		}
		if cleanupUnknown {
			return service.markUnknown(ctx, q, current, code)
		}
		_, err = q.FailHostRemovalOperation(ctx, db.FailHostRemovalOperationParams{ID: current.ID, EnterpriseID: current.EnterpriseID,
			Status: "failed", ErrorCode: pgtype.Text{String: code, Valid: true}, LocalCleanup: "pending"})
		if err != nil {
			return err
		}
		if current.TargetType == TargetBastion {
			_, _ = q.MarkRemovalBastionHostTerminal(ctx, db.MarkRemovalBastionHostTerminalParams{ID: current.HostID, EnterpriseID: current.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true}, RemovalGeneration: current.RemovalGeneration,
				Status: "removal_failed", LocalCleanup: "pending"})
			_, _ = q.MarkRemovalBastionTerminal(ctx, db.MarkRemovalBastionTerminalParams{ID: current.BastionScopeID.UUID, EnterpriseID: current.EnterpriseID,
				ActiveConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true}, RemovalGeneration: current.RemovalGeneration,
				Status: "removal_failed", LocalCleanup: "pending"})
		} else {
			_, _ = q.MarkRemovalHostTerminal(ctx, db.MarkRemovalHostTerminalParams{ID: current.HostID, EnterpriseID: current.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true}, RemovalGeneration: current.RemovalGeneration,
				Status: "removal_failed", LocalCleanup: "pending"})
		}
		_ = service.step(ctx, q, current, current.Stage, "failed", map[string]any{}, code)
		if err := service.recordEvent(ctx, q, current, current.Stage, "failed", code); err != nil {
			return err
		}
		return appendRemovalAudit(ctx, q, current, "host.removal.failed", "failure", code)
	})
}

func (service Service) Retry(ctx context.Context, actorID string, enterpriseID, operationID uuid.UUID, idempotencyKey string) (View, error) {
	operation, err := postgres.ExecuteIdempotent(ctx, service.Store, service.Actions.Idempotency, "enterprise", actorID, "host_removal.retry",
		idempotencyKey, map[string]any{"operation_id": operationID}, 202, func(q *db.Queries) (db.HostRemovalOperation, error) {
			operation, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operationID, EnterpriseID: enterpriseID})
			if err != nil || operation.Status != "failed" && operation.Status != "cleanup_unknown" {
				return db.HostRemovalOperation{}, ErrOperationState
			}
			if err = service.validateOperationFence(ctx, q, operation); err != nil {
				return db.HostRemovalOperation{}, err
			}
			if operation.TargetType == TargetBastion {
				if rows, resumeErr := q.ResumeBastionScopeRemoval(ctx, db.ResumeBastionScopeRemovalParams{ID: operation.BastionScopeID.UUID, EnterpriseID: enterpriseID,
					ActiveConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration}); resumeErr != nil || rows != 1 {
					return db.HostRemovalOperation{}, ErrIdentityChanged
				}
				if rows, resumeErr := q.ResumeBastionRootHostRemoval(ctx, db.ResumeBastionRootHostRemovalParams{ID: operation.HostID, EnterpriseID: enterpriseID,
					ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration}); resumeErr != nil || rows != 1 {
					return db.HostRemovalOperation{}, ErrIdentityChanged
				}
			} else if rows, resumeErr := q.ResumeManagedHostRemoval(ctx, db.ResumeManagedHostRemovalParams{ID: operation.HostID, EnterpriseID: enterpriseID,
				ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration}); resumeErr != nil || rows != 1 {
				return db.HostRemovalOperation{}, ErrIdentityChanged
			}
			status := "queued"
			if operation.DeliveryMethod == "manual" {
				status = "awaiting_manual_execution"
			}
			stage := firstUnverifiedStage(ctx, q, operation)
			if _, err = q.SupersedeHostRemovalCommands(ctx, db.SupersedeHostRemovalCommandsParams{EnterpriseID: enterpriseID, OperationRef: operation.ID.String()}); err != nil {
				return db.HostRemovalOperation{}, err
			}
			operation, err = q.RetryHostRemovalOperation(ctx, db.RetryHostRemovalOperationParams{ID: operation.ID, EnterpriseID: enterpriseID,
				Status: status, Stage: stage, ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(operationTTL), Valid: true}})
			if err != nil {
				return db.HostRemovalOperation{}, err
			}
			_ = service.recordEvent(ctx, q, operation, stage, "resumed", "")
			return operation, nil
		})
	if err != nil {
		return View{}, err
	}
	return service.Get(ctx, enterpriseID, operation.ID)
}

func (service Service) ProcessServerOnly(ctx context.Context, operation db.HostRemovalOperation) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		current, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
		if err != nil || current.Status != "running" || current.RemovalMode != ModeForget {
			return ErrOperationState
		}
		_, _ = q.TerminateRemoteAccessSessionsByHostRemoval(ctx, db.TerminateRemoteAccessSessionsByHostRemovalParams{EnterpriseID: current.EnterpriseID, HostID: current.HostID})
		_, _ = q.InvalidateHostTelemetryForRemoval(ctx, db.InvalidateHostTelemetryForRemovalParams{EnterpriseID: current.EnterpriseID, ResourceID: current.HostID})
		_, _ = q.RevokeRemovalConnectorCommands(ctx, db.RevokeRemovalConnectorCommandsParams{EnterpriseID: current.EnterpriseID, ConnectorID: current.ConnectorID})
		_ = q.RevokeConnectorCertificates(ctx, db.RevokeConnectorCertificatesParams{ConnectorID: current.ConnectorID, EnterpriseID: current.EnterpriseID})
		_ = q.RevokePKISubjectCertificates(ctx, db.RevokePKISubjectCertificatesParams{SubjectKind: "connector", SubjectID: current.ConnectorID.String(), RevocationReason: "force_forgotten"})
		_, _ = q.FenceConnectorForReplacement(ctx, db.FenceConnectorForReplacementParams{ID: current.ConnectorID, EnterpriseID: current.EnterpriseID})
		if current.TargetType == TargetBastion {
			_, _ = q.MarkRemovalBastionHostTerminal(ctx, db.MarkRemovalBastionHostTerminalParams{ID: current.HostID, EnterpriseID: current.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true}, RemovalGeneration: current.RemovalGeneration, Status: "uninstalled", LocalCleanup: "unknown"})
			_, _ = q.MarkRemovalBastionTerminal(ctx, db.MarkRemovalBastionTerminalParams{ID: current.BastionScopeID.UUID, EnterpriseID: current.EnterpriseID,
				ActiveConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true}, RemovalGeneration: current.RemovalGeneration, Status: "uninstalled", LocalCleanup: "unknown"})
			scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: current.BastionScopeID.UUID, EnterpriseID: current.EnterpriseID})
			if err != nil {
				return err
			}
			if _, err = q.DeleteUninstalledBastionHost(ctx, db.DeleteUninstalledBastionHostParams{ID: current.HostID, EnterpriseID: current.EnterpriseID}); err != nil {
				return err
			}
			if _, err = q.DeleteUninstalledBastionScope(ctx, db.DeleteUninstalledBastionScopeParams{ID: scope.ID, EnterpriseID: current.EnterpriseID, ResourceVersion: scope.ResourceVersion}); err != nil {
				return err
			}
		} else {
			_, _ = q.MarkRemovalHostTerminal(ctx, db.MarkRemovalHostTerminalParams{ID: current.HostID, EnterpriseID: current.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: current.ConnectorID, Valid: true}, RemovalGeneration: current.RemovalGeneration, Status: "uninstalled", LocalCleanup: "unknown"})
			host, err := q.GetHost(ctx, db.GetHostParams{ID: current.HostID, EnterpriseID: current.EnterpriseID})
			if err != nil {
				return err
			}
			if _, err = q.DeleteUninstalledManagedHost(ctx, db.DeleteUninstalledManagedHostParams{ID: host.ID, EnterpriseID: current.EnterpriseID, ResourceVersion: host.ResourceVersion}); err != nil {
				return err
			}
		}
		_, err = q.CompleteHostRemovalOperation(ctx, db.CompleteHostRemovalOperationParams{ID: current.ID, EnterpriseID: current.EnterpriseID,
			LocalCleanup: "unknown", ErrorCode: pgtype.Text{String: "LOCAL_CLEANUP_UNKNOWN", Valid: true}})
		if err == nil {
			_ = service.recordEvent(ctx, q, current, "completed", "succeeded", "LOCAL_CLEANUP_UNKNOWN")
			err = appendRemovalAudit(ctx, q, current, "host.removal.force_forget", "success", "LOCAL_CLEANUP_UNKNOWN")
		}
		return err
	})
}

func (service Service) DeleteRecord(ctx context.Context, enterpriseID uuid.UUID, targetType string, targetID uuid.UUID, expectedVersion int64) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		if targetType == TargetManagedHost {
			_, err := q.DeleteUninstalledManagedHost(ctx, db.DeleteUninstalledManagedHostParams{ID: targetID, EnterpriseID: enterpriseID, ResourceVersion: expectedVersion})
			return err
		}
		if targetType != TargetBastion {
			return ErrInvalidTarget
		}
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: targetID, EnterpriseID: enterpriseID})
		if err != nil || !scope.ConnectorHostID.Valid {
			return ErrInvalidTarget
		}
		if _, err = q.DeleteUninstalledBastionHost(ctx, db.DeleteUninstalledBastionHostParams{ID: scope.ConnectorHostID.UUID, EnterpriseID: enterpriseID}); err != nil {
			return err
		}
		_, err = q.DeleteUninstalledBastionScope(ctx, db.DeleteUninstalledBastionScopeParams{ID: targetID, EnterpriseID: enterpriseID, ResourceVersion: expectedVersion})
		return err
	})
}

func (service Service) step(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation, stage, status string, postcondition any, code string) error {
	encoded, _ := json.Marshal(postcondition)
	digest := []byte(nil)
	if status == "succeeded" {
		value := sha256Bytes(encoded)
		digest = value
	}
	_, err := q.UpsertHostRemovalStep(ctx, db.UpsertHostRemovalStepParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID,
		Stage: stage, Status: status, Attempt: operation.Attempts, Postcondition: encoded, ResultHash: digest,
		ErrorCode: pgtype.Text{String: code, Valid: code != ""}})
	return err
}

func firstUnverifiedStage(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation) string {
	steps, err := q.ListHostRemovalSteps(ctx, db.ListHostRemovalStepsParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID})
	if err != nil {
		return "terminating_sessions"
	}
	completed := map[string]bool{}
	for _, step := range steps {
		completed[step.Stage] = step.Status == "succeeded"
	}
	for _, stage := range orderedStages {
		if operation.TargetType != TargetBastion && stage == "stopping_relay" {
			continue
		}
		if !completed[stage] {
			return stage
		}
	}
	return "verifying_cleanup"
}

func (service Service) viewWithQueries(ctx context.Context, q *db.Queries, enterpriseID, operationID uuid.UUID) (View, error) {
	operation, err := q.GetHostRemovalOperation(ctx, db.GetHostRemovalOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil {
		return View{}, err
	}
	events, err := q.ListHostRemovalEvents(ctx, db.ListHostRemovalEventsParams{OperationID: operationID, EnterpriseID: enterpriseID})
	return View{Operation: operation, Events: events}, err
}

func sha256Bytes(value []byte) []byte { digest := sha256.Sum256(value); return digest[:] }

func appendRemovalAudit(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation, action, result, code string) error {
	resourceID := operation.HostID
	if operation.TargetType == TargetBastion && operation.BastionScopeID.Valid {
		resourceID = operation.BastionScopeID.UUID
	}
	_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: operation.EnterpriseID, Valid: true},
		ActorType: "system", ActorID: "host-removal-worker", Action: action, ResourceType: operation.TargetType, ResourceID: resourceID.String(),
		Result: result, Details: map[string]any{"operation_id": operation.ID.String(), "connector_id": operation.ConnectorID.String(), "status": operation.Status, "reason_code": code}})
	return err
}

func (service Service) Run(ctx context.Context, owner string) error {
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = service.Store.Queries.RecoverHostRemovalOperations(ctx)
			expired, _ := service.Store.Queries.ExpireHostRemovalOperations(ctx)
			for _, operation := range expired {
				_ = service.markExpiredTarget(ctx, operation)
			}
			operations, err := service.Store.Queries.ClaimServerOnlyHostRemovalOperations(ctx, db.ClaimServerOnlyHostRemovalOperationsParams{Limit: 8, LeaseOwner: owner})
			if err != nil {
				return err
			}
			for _, operation := range operations {
				if err := service.ProcessServerOnly(ctx, operation); err != nil && !errors.Is(err, ErrOperationState) {
					_ = service.Fail(ctx, operation, "HOST_REMOVAL_SERVER_FINALIZE_FAILED", false)
				}
			}
		}
	}
}

func (service Service) markExpiredTarget(ctx context.Context, operation db.HostRemovalOperation) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		if operation.TargetType == TargetBastion {
			_, _ = q.MarkRemovalBastionHostTerminal(ctx, db.MarkRemovalBastionHostTerminalParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
				Status: "cleanup_unknown", LocalCleanup: "unknown"})
			_, _ = q.MarkRemovalBastionTerminal(ctx, db.MarkRemovalBastionTerminalParams{ID: operation.BastionScopeID.UUID, EnterpriseID: operation.EnterpriseID,
				ActiveConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
				Status: "cleanup_unknown", LocalCleanup: "unknown"})
		} else {
			_, _ = q.MarkRemovalHostTerminal(ctx, db.MarkRemovalHostTerminalParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RemovalGeneration: operation.RemovalGeneration,
				Status: "cleanup_unknown", LocalCleanup: "unknown"})
		}
		return service.recordEvent(ctx, q, operation, operation.Stage, "unknown", "HOST_REMOVAL_EXPIRED")
	})
}
