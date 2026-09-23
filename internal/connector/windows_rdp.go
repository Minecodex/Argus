package connector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var (
	ErrWindowsRDPUnavailable = errors.New("WINDOWS_RDP_UNAVAILABLE")
	ErrWindowsRDPEnabled     = errors.New("WINDOWS_RDP_ALREADY_ENABLED")
)

type windowsRDPEnablePlan struct {
	HostID                uuid.UUID `json:"host_id"`
	HostResourceVersion   int64     `json:"host_resource_version"`
	ConnectorID           uuid.UUID `json:"connector_id"`
	ConnectorVersion      int64     `json:"connector_version"`
	ConnectionEpoch       int64     `json:"connection_epoch"`
	ObservationObservedAt time.Time `json:"observation_observed_at"`
	EnforceNLA            bool      `json:"enforce_nla"`
	EnableFirewall        bool      `json:"enable_firewall"`
}

func (service BastionService) PreviewWindowsRDPEnable(ctx context.Context, subject resource.Subject, enterpriseID, hostID uuid.UUID, expectedVersion int64, idempotencyKey string) (db.PendingAction, error) {
	host, err := service.Store.Queries.GetHost(ctx, db.GetHostParams{ID: hostID, EnterpriseID: enterpriseID})
	if err != nil || host.ResourceVersion != expectedVersion || host.Role != "managed_host" || host.Platform != "windows" || !host.ConnectorID.Valid {
		return db.PendingAction{}, ErrWindowsRDPUnavailable
	}
	observation, err := service.Store.Queries.GetHostRuntimeObservation(ctx, db.GetHostRuntimeObservationParams{EnterpriseID: enterpriseID, HostID: hostID})
	if err != nil || observation.ConnectorID != host.ConnectorID.UUID || time.Since(observation.ObservedAt.Time) > 2*time.Minute {
		return db.PendingAction{}, ErrWindowsRDPUnavailable
	}
	if observation.RdpStatus == "enabled" && observation.RdpNlaEnabled && observation.RdpFirewallEnabled && observation.RdpServiceRunning {
		return db.PendingAction{}, ErrWindowsRDPEnabled
	}
	connector, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: host.ConnectorID.UUID, EnterpriseID: enterpriseID})
	if err != nil || connector.Status != "online" || connector.ConnectionEpoch < 1 {
		return db.PendingAction{}, ErrWindowsRDPUnavailable
	}
	plan := windowsRDPEnablePlan{HostID: host.ID, HostResourceVersion: host.ResourceVersion, ConnectorID: connector.ID,
		ConnectorVersion: connector.Version, ConnectionEpoch: connector.ConnectionEpoch, ObservationObservedAt: observation.ObservedAt.Time,
		EnforceNLA: true, EnableFirewall: true}
	return service.Actions.Prepare(ctx, subject.ActorID, enterpriseID, resource.PrepareActionInput{RunID: subject.RunID, ActionType: "host.windows_rdp.enable", Title: "Enable Windows RDP",
		Summary: "Enable Remote Desktop, NLA, the TermService service, and the built-in RDP firewall rules", Risk: "dangerous", ResourceType: "host",
		ResourceID: uuid.NullUUID{UUID: host.ID, Valid: true}, ExpectedResourceVersion: pgtype.Int8{Int64: host.ResourceVersion, Valid: true},
		AuthorizationVersion: subject.AuthorizationVersion, Preview: map[string]any{"host_id": host.ID, "registry": []string{"fDenyTSConnections=0", "UserAuthentication=1"},
			"service": "TermService", "firewall_rules": []string{"RemoteDesktop-UserMode-In-TCP", "RemoteDesktop-UserMode-In-UDP"}},
		Diff: []map[string]string{{"kind": "change", "text": "Enable Windows Remote Desktop with NLA and built-in firewall rules"}}, ImmutablePlan: plan,
		ResourceScopeSnapshot: resource.NewResourceAuthorizationSnapshot("host", host.ID), CommitHandler: "argus.host.windows_rdp.enable.commit"}, idempotencyKey)
}

func (service BastionService) revalidateWindowsRDPEnable(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, plan windowsRDPEnablePlan) error {
	host, err := q.GetHost(ctx, db.GetHostParams{ID: plan.HostID, EnterpriseID: enterpriseID})
	if err != nil || host.ResourceVersion != plan.HostResourceVersion || host.Platform != "windows" || !host.ConnectorID.Valid || host.ConnectorID.UUID != plan.ConnectorID {
		return resource.ErrActionInvalidated
	}
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: plan.ConnectorID, EnterpriseID: enterpriseID})
	if err != nil || connector.Version != plan.ConnectorVersion || connector.ConnectionEpoch != plan.ConnectionEpoch || connector.Status != "online" {
		return resource.ErrActionInvalidated
	}
	observation, err := q.GetHostRuntimeObservation(ctx, db.GetHostRuntimeObservationParams{EnterpriseID: enterpriseID, HostID: plan.HostID})
	if err != nil || !observation.ObservedAt.Time.Equal(plan.ObservationObservedAt) || observation.RdpStatus == "enabled" {
		return resource.ErrActionInvalidated
	}
	return nil
}

func (service BastionService) commitWindowsRDPEnable(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	var plan windowsRDPEnablePlan
	if json.Unmarshal(raw, &plan) != nil || !plan.EnforceNLA || !plan.EnableFirewall || service.revalidateWindowsRDPEnable(ctx, q, action.EnterpriseID, plan) != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	payload, err := json.Marshal(connectorv1.HostWindowsRDPConfigure{HostId: plan.HostID.String(), Enable: true, EnforceNla: true, EnableFirewall: true})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	hash := sha256.Sum256(payload)
	commandID, err := randomID("cmd_")
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	command, err := q.CreateConnectorCommand(ctx, db.CreateConnectorCommandParams{ID: newID(), CommandID: commandID, EnterpriseID: action.EnterpriseID,
		ConnectorID: plan.ConnectorID, ConnectionEpoch: plan.ConnectionEpoch, OperationRef: action.ActionRef, CommandType: "host_windows_rdp_configure",
		PayloadSchemaVersion: "argus.host_windows_rdp_configure/v1", Payload: payload, PayloadHash: hash[:], IdempotencyKey: action.ActionRef,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(5 * time.Minute), Valid: true}})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	service.Enrollment.NotifyConnectorCommand(ctx, plan.ConnectorID, plan.ConnectionEpoch)
	return resource.ActionCommitResult{ResourceType: "host", ResourceID: plan.HostID, ResourceVersion: plan.HostResourceVersion,
		Summary: "Windows RDP enablement queued", ConnectorCommandID: uuid.NullUUID{UUID: command.ID, Valid: true}}, nil
}
