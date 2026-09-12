package hostremoval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/installinstruction"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/trustbundle"
)

const (
	operationTTL = 30 * time.Minute
	tokenTTL     = 15 * time.Minute
)

type Service struct {
	Store        *postgres.Store
	Actions      resource.PendingActionService
	Access       resource.AccessService
	TrustBundles trustbundle.Service
	TokenKey     []byte
	ExternalURL  string
	Next         resource.ActionExtension
}

type View struct {
	Operation db.HostRemovalOperation
	Events    []db.HostRemovalOperationEvent
}

type Instruction struct {
	OperationID uuid.UUID
	Set         installinstruction.Set
}

func (service Service) Preview(ctx context.Context, subject resource.Subject, enterpriseID uuid.UUID, input PreviewInput, idempotencyKey string) (db.PendingAction, error) {
	plan, snapshot, err := service.freeze(ctx, service.Store.Queries, subject, enterpriseID, input)
	if err != nil {
		return db.PendingAction{}, err
	}
	risk := "dangerous"
	title := "Uninstall Host"
	summary := "Stop Argus-managed workloads and uninstall the Connector"
	if plan.TargetType == TargetBastion {
		title = "Uninstall Bastion"
		summary = "Drain the Bastion, stop Relay, and uninstall Argus-managed software"
	}
	if plan.Mode == ModeForget {
		risk, title, summary = "critical", "Remove unreachable resource from Argus", "Revoke server identities without verified local cleanup"
	}
	return service.Actions.Prepare(ctx, subject.ActorID, enterpriseID, resource.PrepareActionInput{
		ActionType: "host.removal." + plan.Mode, Title: title, Summary: summary, Risk: risk,
		ResourceType: plan.TargetType, ResourceID: uuid.NullUUID{UUID: plan.TargetID, Valid: true},
		ExpectedResourceVersion: pgtype.Int8{Int64: plan.ExpectedVersion, Valid: true}, AuthorizationVersion: subject.AuthorizationVersion,
		Preview: map[string]any{"target_type": plan.TargetType, "target_id": plan.TargetID, "name": plan.Name, "mode": plan.Mode,
			"delivery_method": plan.DeliveryMethod, "ssh_path": plan.SSHPath, "stages": stagesFor(plan), "dependencies": plan.Dependencies},
		Diff: []map[string]string{{"kind": "change", "text": "status: active -> " + removalPreviewStatus(plan.Mode)},
			{"kind": "change", "text": "local_cleanup: verified -> " + localCleanupPreview(plan.Mode)}},
		ImmutablePlan: plan, ResourceScopeSnapshot: snapshot, CommitHandler: "host.removal.commit", RunID: subject.RunID,
	}, idempotencyKey)
}

func (service Service) freeze(ctx context.Context, q *db.Queries, subject resource.Subject, enterpriseID uuid.UUID, input PreviewInput) (Plan, impactSnapshot, error) {
	if input.TargetID == uuid.Nil || input.ExpectedVersion < 1 || input.Mode != ModeUninstall && input.Mode != ModeForget {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if input.TargetType == TargetManagedHost {
		return service.freezeHost(ctx, q, subject, enterpriseID, input)
	}
	if input.TargetType == TargetBastion {
		return service.freezeBastion(ctx, q, subject, enterpriseID, input)
	}
	return Plan{}, impactSnapshot{}, ErrInvalidTarget
}

func (service Service) freezeHost(ctx context.Context, q *db.Queries, subject resource.Subject, enterpriseID uuid.UUID, input PreviewInput) (Plan, impactSnapshot, error) {
	if !service.Access.CanAccess(subject.AuthorizedResourceIDs, input.TargetID) {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	host, err := q.GetHost(ctx, db.GetHostParams{ID: input.TargetID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	if host.Role != TargetManagedHost {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if host.ResourceVersion != input.ExpectedVersion {
		return Plan{}, impactSnapshot{}, resource.ErrVersionConflict
	}
	if !host.ConnectorID.Valid {
		return Plan{}, impactSnapshot{}, ErrNotInstalled
	}
	if input.Mode == ModeUninstall && (host.Status == "removal_failed" || host.Status == "cleanup_unknown") {
		return Plan{}, impactSnapshot{}, ErrOperationState
	}
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: host.ConnectorID.UUID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, impactSnapshot{}, ErrIdentityChanged
	}
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	if connector.Role != "host" || connector.HostID.UUID != host.ID || connector.ConnectionEpoch < 1 {
		return Plan{}, impactSnapshot{}, ErrIdentityChanged
	}
	if input.Mode == ModeForget && input.ConfirmationName != host.Name {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	dependencies, err := q.ListHostRemovalDependencies(ctx, db.ListHostRemovalDependenciesParams{EnterpriseID: enterpriseID, HostID: host.ID})
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	items := make([]Dependency, 0, len(dependencies))
	for _, value := range dependencies {
		items = append(items, Dependency{Type: value.DependencyType, ID: value.DependencyID, Name: value.Name, Reason: value.Reason})
	}
	plan := Plan{SchemaVersion: "argus.host_removal/v1", TargetType: TargetManagedHost, TargetID: host.ID, HostID: host.ID,
		BastionScopeID: host.BastionScopeID, Name: host.Name, Mode: input.Mode, ExpectedVersion: host.ResourceVersion, ConnectorID: connector.ID, ConnectorVersion: connector.Version,
		ConnectionEpoch: connector.ConnectionEpoch, RemovalGeneration: host.RemovalGeneration + 1, TargetPlatform: host.Platform + "_" + host.Architecture.String,
		ControlPath: host.ControlPath, Dependencies: items, ComponentInventory: mustJSON(items), CreatedAt: time.Now().UTC()}
	if !validTargetPlatform(plan.TargetPlatform) {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if err = service.applyDelivery(ctx, q, enterpriseID, host, input, &plan); err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	if host.Platform == "windows" {
		journal, journalErr := q.GetActiveHostManagedChangeJournal(ctx, db.GetActiveHostManagedChangeJournalParams{EnterpriseID: enterpriseID, HostID: host.ID, ChangeType: "windows_rdp"})
		if journalErr == nil {
			plan.ManagedChangeID = uuid.NullUUID{UUID: journal.ID, Valid: true}
			plan.ManagedChangeBefore = append(json.RawMessage(nil), journal.BeforeState...)
			plan.ManagedChangeApplied = append(json.RawMessage(nil), journal.AppliedState...)
		} else if !errors.Is(journalErr, pgx.ErrNoRows) {
			return Plan{}, impactSnapshot{}, journalErr
		}
	}
	if err = service.applyTrust(ctx, &plan); err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	snapshot := impactSnapshot{TargetType: plan.TargetType, TargetID: plan.TargetID, ResourceVersion: host.ResourceVersion,
		ConnectorID: connector.ID, ConnectorVersion: connector.Version, ConnectionEpoch: connector.ConnectionEpoch,
		RemovalGeneration: host.RemovalGeneration, Dependencies: items}
	return plan, snapshot, nil
}

func (service Service) freezeBastion(ctx context.Context, q *db.Queries, subject resource.Subject, enterpriseID uuid.UUID, input PreviewInput) (Plan, impactSnapshot, error) {
	scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: input.TargetID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	if !scope.ConnectorHostID.Valid || !service.Access.CanAccess(subject.AuthorizedResourceIDs, scope.ConnectorHostID.UUID) {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if scope.ResourceVersion != input.ExpectedVersion {
		return Plan{}, impactSnapshot{}, resource.ErrVersionConflict
	}
	if !scope.ActiveConnectorID.Valid {
		return Plan{}, impactSnapshot{}, ErrNotInstalled
	}
	if input.Mode == ModeUninstall && (scope.Status == "removal_failed" || scope.Status == "cleanup_unknown") {
		return Plan{}, impactSnapshot{}, ErrOperationState
	}
	host, err := q.GetHost(ctx, db.GetHostParams{ID: scope.ConnectorHostID.UUID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, impactSnapshot{}, ErrIdentityChanged
	}
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	if host.Role != "bastion" || !host.ConnectorID.Valid || host.ConnectorID.UUID != scope.ActiveConnectorID.UUID {
		return Plan{}, impactSnapshot{}, ErrIdentityChanged
	}
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: scope.ActiveConnectorID.UUID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, impactSnapshot{}, ErrIdentityChanged
	}
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	if connector.Role != "bastion" || connector.ConnectionEpoch < 1 {
		return Plan{}, impactSnapshot{}, ErrIdentityChanged
	}
	if input.Mode == ModeForget && input.ConfirmationName != scope.Name {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	dependencies, err := q.ListBastionRemovalDependencies(ctx, db.ListBastionRemovalDependenciesParams{EnterpriseID: enterpriseID, BastionScopeID: uuid.NullUUID{UUID: scope.ID, Valid: true}, ResourceID: host.ID})
	if err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	items := make([]Dependency, 0, len(dependencies))
	for _, value := range dependencies {
		items = append(items, Dependency{Type: value.DependencyType, ID: value.DependencyID, Name: value.Name, Reason: value.Reason})
	}
	plan := Plan{SchemaVersion: "argus.host_removal/v1", TargetType: TargetBastion, TargetID: scope.ID, HostID: host.ID,
		BastionScopeID: uuid.NullUUID{UUID: scope.ID, Valid: true}, Name: scope.Name, Mode: input.Mode, ExpectedVersion: scope.ResourceVersion,
		ConnectorID: connector.ID, ConnectorVersion: connector.Version, ConnectionEpoch: connector.ConnectionEpoch,
		RemovalGeneration: scope.RemovalGeneration + 1, TargetPlatform: "linux_" + host.Architecture.String, ControlPath: host.ControlPath,
		RelayHTTPSPort: int32(scope.RelayHttpsPort), RelayGatewayPort: int32(scope.RelayGatewayPort),
		Dependencies: items, ComponentInventory: mustJSON(items), CreatedAt: time.Now().UTC()}
	if !validTargetPlatform(plan.TargetPlatform) {
		return Plan{}, impactSnapshot{}, ErrInvalidTarget
	}
	if input.Mode == ModeForget {
		if scope.Status != "offline" && scope.Status != "removal_failed" && scope.Status != "cleanup_unknown" {
			return Plan{}, impactSnapshot{}, ErrOperationState
		}
		plan.DeliveryMethod, plan.SSHPath = "server_only", "none"
	} else {
		origin, originErr := q.GetLatestConnectorInstallOperation(ctx, db.GetLatestConnectorInstallOperationParams{
			ConnectorID: connector.ID, EnterpriseID: enterpriseID,
		})
		if errors.Is(originErr, pgx.ErrNoRows) && scope.OnboardingMode == "command" {
			plan.DeliveryMethod, plan.SSHPath = "manual", "none"
		} else {
			if originErr != nil || origin.HostID != host.ID || origin.BastionScopeID != scope.ID || origin.ConnectorID != connector.ID {
				return Plan{}, impactSnapshot{}, ErrOperationState
			}
			plan.DeliveryMethod, plan.SSHPath = "ssh", "direct_executor"
			if err = service.applyConnectionTest(ctx, q, enterpriseID, host, input, &plan); err != nil {
				return Plan{}, impactSnapshot{}, err
			}
		}
	}
	if err = service.applyTrust(ctx, &plan); err != nil {
		return Plan{}, impactSnapshot{}, err
	}
	snapshot := impactSnapshot{TargetType: plan.TargetType, TargetID: plan.TargetID, ResourceVersion: scope.ResourceVersion,
		ConnectorID: connector.ID, ConnectorVersion: connector.Version, ConnectionEpoch: connector.ConnectionEpoch,
		RemovalGeneration: scope.RemovalGeneration, Dependencies: items}
	return plan, snapshot, nil
}

func (service Service) applyDelivery(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, host db.Host, input PreviewInput, plan *Plan) error {
	if input.Mode == ModeForget {
		if host.ConnectionStatus != "offline" && host.Status != "removal_failed" && host.Status != "cleanup_unknown" {
			return ErrOperationState
		}
		plan.DeliveryMethod, plan.SSHPath = "server_only", "none"
		return nil
	}
	origin, err := q.GetLatestHostOnboardingOperationByConnector(ctx, db.GetLatestHostOnboardingOperationByConnectorParams{
		HostID: host.ID, ConnectorID: plan.ConnectorID, EnterpriseID: enterpriseID,
	})
	if err != nil {
		return ErrOperationState
	}
	plan.DeliveryMethod, plan.SSHPath = origin.InstallMethod, origin.SshPath
	if origin.InstallMethod == "manual" {
		plan.SSHPath = "none"
		var original struct {
			HTTPSDialAddress  string `json:"https_dial_address"`
			EnrollDialAddress string `json:"enroll_dial_address"`
		}
		_ = json.Unmarshal(origin.Plan, &original)
		plan.HTTPSDialAddress = original.HTTPSDialAddress
		if plan.HTTPSDialAddress == "" && plan.ControlPath == "bastion_relay" {
			plan.HTTPSDialAddress = original.EnrollDialAddress
		}
		return nil
	}
	if origin.InstallMethod != "ssh" || origin.SshPath != "direct_executor" && origin.SshPath != "bastion_connector" {
		return ErrOperationState
	}
	return service.applyConnectionTest(ctx, q, enterpriseID, host, input, plan)
}

func (service Service) applyConnectionTest(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, host db.Host, input PreviewInput, plan *Plan) error {
	if !input.ConnectionTestID.Valid || !input.CredentialID.Valid {
		return ErrConnectionTestNeeded
	}
	test, err := q.GetConnectionTest(ctx, db.GetConnectionTestParams{ID: input.ConnectionTestID.UUID, EnterpriseID: enterpriseID})
	if err != nil || test.TargetType != "host" || test.Status != "succeeded" || time.Now().UTC().After(test.ExpiresAt.Time) ||
		!test.CredentialID.Valid || test.CredentialID.UUID != input.CredentialID.UUID || !test.CredentialVersion.Valid {
		return ErrConnectionTestNeeded
	}
	var connection connectionSnapshot
	var evidence connectionEvidence
	if json.Unmarshal(test.RequestPlan, &connection) != nil || json.Unmarshal(test.Result, &evidence) != nil ||
		connection.Address != host.Address.String || connection.Port != host.Port || connection.Platform != host.Platform ||
		connection.SSHPath != plan.SSHPath || connection.CredentialID.UUID != input.CredentialID.UUID ||
		evidence.HostKeyFingerprint == "" || evidence.Platform != host.Platform || evidence.Architecture != host.Architecture.String || !evidence.Privileged {
		return ErrConnectionTestNeeded
	}
	if plan.SSHPath == "bastion_connector" && (!host.BastionScopeID.Valid || connection.BastionScopeID != host.BastionScopeID) {
		return ErrConnectionTestNeeded
	}
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: input.CredentialID.UUID, EnterpriseID: enterpriseID})
	if err != nil || credential.Status != "active" || credential.Protocol != "ssh" || credential.Version != test.CredentialVersion.Int64 {
		return ErrConnectionTestNeeded
	}
	plan.ConnectionTestID, plan.CredentialID = input.ConnectionTestID, input.CredentialID
	plan.CredentialVersion, plan.Address, plan.Port, plan.Username = credential.Version, connection.Address, connection.Port, connection.Username
	plan.PinnedHostKey = evidence.HostKeyFingerprint
	return nil
}

func (service Service) applyTrust(ctx context.Context, plan *Plan) error {
	bundle, err := service.TrustBundles.Current(ctx)
	if err != nil || bundle.Epoch < 1 {
		return ErrOperationState
	}
	plan.TrustBundleEpoch = bundle.Epoch
	return nil
}

func (service Service) RevalidateAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) ([]byte, error) {
	if !strings.HasPrefix(action.ActionType, "host.removal.") {
		if service.Next != nil {
			return service.Next.RevalidateAction(ctx, q, action, raw)
		}
		return nil, resource.ErrActionInvalidated
	}
	var frozen Plan
	if json.Unmarshal(raw, &frozen) != nil || frozen.SchemaVersion != "argus.host_removal/v1" {
		return nil, resource.ErrActionInvalidated
	}
	current, snapshot, err := service.freeze(ctx, q, resource.Subject{ActorID: action.CreatorSubjectID.String(), ActorType: action.CreatorSubjectType,
		AuthorizationVersion: action.AuthorizationVersion, AuthorizedResourceIDs: []uuid.UUID{frozen.HostID}}, action.EnterpriseID,
		PreviewInput{TargetType: frozen.TargetType, TargetID: frozen.TargetID, ExpectedVersion: frozen.ExpectedVersion, Mode: frozen.Mode,
			ConnectionTestID: frozen.ConnectionTestID, CredentialID: frozen.CredentialID, ConfirmationName: frozen.Name})
	if err != nil || !plansEqual(frozen, current) {
		return nil, resource.ErrActionInvalidated
	}
	if current.TargetType == TargetBastion && len(current.Dependencies) > 0 {
		return nil, DependencyError{Items: current.Dependencies}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(encoded)
	return hash[:], nil
}

func (service Service) CommitAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	if !strings.HasPrefix(action.ActionType, "host.removal.") {
		if service.Next != nil {
			return service.Next.CommitAction(ctx, q, action, raw)
		}
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	var plan Plan
	if json.Unmarshal(raw, &plan) != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	if _, err := service.RevalidateAction(ctx, q, action, raw); err != nil {
		return resource.ActionCommitResult{}, err
	}
	resourceVersion, generation, err := service.startTarget(ctx, q, action.EnterpriseID, plan)
	if err != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	_, _ = q.RevokeActiveHostEnrollmentTokens(ctx, db.RevokeActiveHostEnrollmentTokensParams{EnterpriseID: action.EnterpriseID, PreallocatedHostID: uuid.NullUUID{UUID: plan.HostID, Valid: true}})
	_, _ = q.RevokeRemovalConnectorCommands(ctx, db.RevokeRemovalConnectorCommandsParams{EnterpriseID: action.EnterpriseID, ConnectorID: plan.ConnectorID})
	_, _ = q.RevokeHostRemovalCredentialLeases(ctx, db.RevokeHostRemovalCredentialLeasesParams{EnterpriseID: action.EnterpriseID, TargetResourceID: plan.HostID})
	plan.RemovalGeneration = generation
	if plan.Mode == ModeForget {
		_, _ = q.SupersedeHostRemovalOperations(ctx, db.SupersedeHostRemovalOperationsParams{EnterpriseID: action.EnterpriseID, HostID: plan.HostID})
	}
	operationID := uuid.New()
	plan.OperationID = uuid.NullUUID{UUID: operationID, Valid: true}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	hash := sha256.Sum256(encoded)
	status, stage := "queued", "queued"
	if plan.DeliveryMethod == "manual" {
		status, stage = "awaiting_manual_execution", "awaiting_manual_execution"
	}
	expires := time.Now().UTC().Add(operationTTL)
	operation, err := q.CreateHostRemovalOperation(ctx, db.CreateHostRemovalOperationParams{ID: operationID, EnterpriseID: action.EnterpriseID,
		PendingActionID: action.ID, TargetType: plan.TargetType, HostID: plan.HostID, BastionScopeID: plan.BastionScopeID,
		ConnectorID: plan.ConnectorID, RemovalMode: plan.Mode, DeliveryMethod: plan.DeliveryMethod, SshPath: plan.SSHPath,
		TargetPlatform: plan.TargetPlatform, ControlPath: plan.ControlPath, ConnectionTestID: plan.ConnectionTestID,
		CredentialID: plan.CredentialID, CredentialVersion: nullableInt(plan.CredentialVersion), PinnedHostKey: plan.PinnedHostKey,
		ResourceVersion: resourceVersion, ConnectorVersion: plan.ConnectorVersion, ConnectionEpoch: plan.ConnectionEpoch,
		RemovalGeneration: generation, TrustBundleEpoch: plan.TrustBundleEpoch, Plan: encoded, PlanHash: hash[:], Status: status, Stage: stage,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	_, _ = q.TerminateRemoteAccessSessionsByHostRemoval(ctx, db.TerminateRemoteAccessSessionsByHostRemovalParams{EnterpriseID: action.EnterpriseID, HostID: plan.HostID})
	_ = service.step(ctx, q, operation, "draining", "succeeded", map[string]any{"new_work_blocked": true}, "")
	_ = service.recordEvent(ctx, q, operation, "draining", "succeeded", "")
	_ = service.step(ctx, q, operation, "terminating_sessions", "succeeded", map[string]any{"sessions_terminating": true}, "")
	_ = service.recordEvent(ctx, q, operation, "terminating_sessions", "succeeded", "")
	_ = service.recordEvent(ctx, q, operation, stage, "started", "")
	result := resource.ActionCommitResult{ResourceType: plan.TargetType, ResourceID: plan.HostID, ResourceVersion: resourceVersion,
		Summary: "Host removal operation created", HostRemovalOperationID: uuid.NullUUID{UUID: operation.ID, Valid: true}}
	if plan.DeliveryMethod == "manual" {
		instruction, err := service.createInstruction(ctx, q, operation)
		if err != nil {
			return resource.ActionCommitResult{}, err
		}
		result.OneTimeCommand = &resource.OneTimeCommandResult{InstructionSets: []installinstruction.Set{instruction.Set}, ExpiresAt: instruction.Set.ExpiresAt}
		result.OneTimeResultKind = "host_removal_command"
	}
	return result, nil
}

func (service Service) startTarget(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, plan Plan) (int64, int64, error) {
	if plan.TargetType == TargetManagedHost {
		host, err := q.StartManagedHostRemoval(ctx, db.StartManagedHostRemovalParams{ID: plan.HostID, EnterpriseID: enterpriseID,
			ResourceVersion: plan.ExpectedVersion, ConnectorID: uuid.NullUUID{UUID: plan.ConnectorID, Valid: true}})
		return host.ResourceVersion, host.RemovalGeneration, err
	}
	scope, err := q.StartBastionScopeRemoval(ctx, db.StartBastionScopeRemovalParams{ID: plan.TargetID, EnterpriseID: enterpriseID,
		ResourceVersion: plan.ExpectedVersion, ActiveConnectorID: uuid.NullUUID{UUID: plan.ConnectorID, Valid: true}})
	if err != nil {
		return 0, 0, err
	}
	if _, err = q.StartBastionRootHostRemoval(ctx, db.StartBastionRootHostRemovalParams{ID: plan.HostID, EnterpriseID: enterpriseID,
		ConnectorID: uuid.NullUUID{UUID: plan.ConnectorID, Valid: true}, RemovalGeneration: scope.RemovalGeneration}); err != nil {
		return 0, 0, err
	}
	return scope.ResourceVersion, scope.RemovalGeneration, nil
}

func (service Service) Get(ctx context.Context, enterpriseID, operationID uuid.UUID) (View, error) {
	operation, err := service.Store.Queries.GetHostRemovalOperation(ctx, db.GetHostRemovalOperationParams{ID: operationID, EnterpriseID: enterpriseID})
	if err != nil {
		return View{}, err
	}
	events, err := service.Store.Queries.ListHostRemovalEvents(ctx, db.ListHostRemovalEventsParams{OperationID: operation.ID, EnterpriseID: enterpriseID})
	return View{Operation: operation, Events: events}, err
}

func (service Service) recordEvent(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation, stage, status, code string) error {
	sequence, err := q.NextHostRemovalEventSequence(ctx, operation.ID)
	if err != nil {
		return err
	}
	_, err = q.CreateHostRemovalEvent(ctx, db.CreateHostRemovalEventParams{ID: uuid.New(), OperationID: operation.ID,
		EnterpriseID: operation.EnterpriseID, Sequence: int64(sequence), Stage: stage, Status: status,
		ErrorCode: pgtype.Text{String: code, Valid: code != ""}})
	return err
}

func (service Service) validateOperationFence(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation) error {
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
	// Connection epochs fence streams, not machine ownership. Reconnection and
	// certificate rotation must not invalidate an already committed removal.
	if err != nil || !removalConnectorMatches(operation, connector) {
		return ErrIdentityChanged
	}
	host, err := q.GetHost(ctx, db.GetHostParams{ID: operation.HostID, EnterpriseID: operation.EnterpriseID})
	if err != nil || !host.ConnectorID.Valid || host.ConnectorID.UUID != operation.ConnectorID || host.RemovalGeneration != operation.RemovalGeneration ||
		!removalTargetStatus(host.Status) || host.Platform+"_"+host.Architecture.String != operation.TargetPlatform {
		return ErrIdentityChanged
	}
	if operation.TargetType == TargetManagedHost {
		return nil
	}
	scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: operation.BastionScopeID.UUID, EnterpriseID: operation.EnterpriseID})
	if err != nil || !scope.ActiveConnectorID.Valid || scope.ActiveConnectorID.UUID != operation.ConnectorID || scope.RemovalGeneration != operation.RemovalGeneration ||
		!scope.ConnectorHostID.Valid || scope.ConnectorHostID.UUID != operation.HostID || !removalTargetStatus(scope.Status) {
		return ErrIdentityChanged
	}
	return nil
}

func removalConnectorMatches(operation db.HostRemovalOperation, connector db.Connector) bool {
	role := "host"
	if operation.TargetType == TargetBastion {
		role = "bastion"
	}
	return connector.ID == operation.ConnectorID && connector.EnterpriseID == operation.EnterpriseID &&
		connector.HostID.Valid && connector.HostID.UUID == operation.HostID && connector.Role == role &&
		connector.Status != "revoked" && connector.Status != "uninstalled" &&
		connector.Version >= operation.ConnectorVersion && connector.ConnectionEpoch >= operation.ConnectionEpoch
}

func removalTargetStatus(status string) bool {
	return status == "draining" || status == "uninstalling" || status == "removal_failed" || status == "cleanup_unknown"
}

func plansEqual(left, right Plan) bool {
	left.CreatedAt, right.CreatedAt = time.Time{}, time.Time{}
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}

func stagesFor(plan Plan) []string {
	stages := []string{"draining", "terminating_sessions", "uninstalling_workloads"}
	if plan.TargetType == TargetBastion {
		stages = append(stages, "stopping_relay")
	}
	if plan.Mode == ModeUninstall {
		stages = append(stages, "uninstalling_connector", "verifying_cleanup")
	}
	return append(stages, "revoking_identities", "completed")
}

func removalPreviewStatus(mode string) string {
	if mode == ModeForget {
		return "deleted"
	}
	return "uninstalled"
}

func localCleanupPreview(mode string) string {
	if mode == ModeForget {
		return "unknown"
	}
	return "verified"
}

func nullableInt(value int64) pgtype.Int8 { return pgtype.Int8{Int64: value, Valid: value > 0} }
func mustJSON(value any) json.RawMessage  { encoded, _ := json.Marshal(value); return encoded }

type DependencyError struct{ Items []Dependency }

func (value DependencyError) Error() string { return ErrDependenciesExist.Error() }
func (value DependencyError) Unwrap() error { return ErrDependenciesExist }

func (service Service) bootstrapURL(operationID uuid.UUID) (string, error) {
	parsed, err := url.Parse(service.ExternalURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("host removal external URL must be HTTPS")
	}
	parsed.Path = "/api/v1/host-removal/bootstrap-script"
	query := parsed.Query()
	query.Set("operation_id", operationID.String())
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (service Service) receiptURL() (string, error) {
	parsed, err := url.Parse(service.ExternalURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("host removal external URL must be HTTPS")
	}
	parsed.Path = "/api/v1/host-removal/receipt"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func dialArgument(value string) string {
	if value == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(value); err != nil {
		return ""
	}
	return value
}

func validTargetPlatform(value string) bool {
	return value == "linux_amd64" || value == "linux_arm64" || value == "windows_amd64"
}

var _ resource.ActionExtension = Service{}
