package remoteexecution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Transport interface {
	Call(context.Context, db.RemoteExecutionIdentity, string, proto.Message, string, time.Duration, func(context.Context, *db.Queries, string) error) (json.RawMessage, error)
}
type CommandNotifier interface {
	NotifyConnectorCommand(context.Context, uuid.UUID, int64)
}
type ConnectorTransport struct {
	Store    *postgres.Store
	Secrets  secret.Service
	Notifier CommandNotifier
}
type transferAttemptKey struct{}
type unsentCommandError struct{ code Error }

func (e unsentCommandError) Error() string { return string(e.code) }
func (e unsentCommandError) Code() string  { return string(e.code) }

func (unsentCommandError) NotDispatched() bool { return true }

func (t ConnectorTransport) Call(ctx context.Context, identity db.RemoteExecutionIdentity, kind string, payload proto.Message, key string, timeout time.Duration, queued func(context.Context, *db.Queries, string) error) (json.RawMessage, error) {
	// Only idempotent file/control cleanup retries receive a new transport ID.
	// The command batch key is immutable across all worker attempts.
	retryable := kind == "remote_execution_file"
	if c, ok := payload.(*connectorv1.RemoteExecutionControl); ok && (c.Operation == "reclaim" || c.Operation == "recover_files") {
		retryable = true
	}
	if retryable {
		if attempt, ok := ctx.Value(transferAttemptKey{}).(int32); ok {
			key += fmt.Sprintf("/transfer-attempt/%d", attempt)
		}
	}
	var binding *connectorv1.RemoteExecutionBatch
	switch value := payload.(type) {
	case *connectorv1.RemoteExecutionBatch:
		binding = value
	case *connectorv1.RemoteExecutionControl:
		binding = value.Binding
	}
	if binding != nil && identity.BindingSource == "ssh_onboarding" {
		operation, e := t.Store.Queries.GetLatestHostOnboardingOperation(ctx, db.GetLatestHostOnboardingOperationParams{HostID: identity.ResourceID, EnterpriseID: identity.EnterpriseID})
		var plan struct {
			Address       string `json:"address"`
			Port          uint32 `json:"port"`
			Username      string `json:"username"`
			PinnedHostKey string `json:"pinned_host_key"`
		}
		if e != nil || json.Unmarshal(operation.Plan, &plan) != nil || plan.Username != identity.Username || plan.PinnedHostKey == "" {
			return nil, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
		binding.SshAddress = plan.Address
		binding.SshPort = plan.Port
		binding.SshHostKey = plan.PinnedHostKey
	}
	if binding != nil && identity.Platform == "kubernetes" {
		cluster, e := t.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: identity.ResourceID, EnterpriseID: identity.EnterpriseID})
		if e != nil || cluster.ResourceVersion != identity.ResourceVersion {
			return nil, Error("REMOTE_IDENTITY_CHANGED")
		}
		binding.KubernetesApiServer = cluster.ApiServer
	}
	if !identity.ConnectorID.Valid {
		if identity.Platform != "kubernetes" || !identity.CredentialID.Valid {
			return nil, Error("REMOTE_ROUTE_UNAVAILABLE")
		}
		return t.callDirect(ctx, identity, kind, payload, key, timeout, queued)
	}
	encoded, err := protojson.Marshal(payload)
	if err != nil {
		return nil, err
	}
	encoded, err = resource.CanonicalJSON(encoded)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 1<<20 {
		return nil, Error("REMOTE_INPUT_INVALID")
	}
	id := uuid.NewSHA1(identity.ID, []byte(key))
	commandID := "remote_" + id.String()
	hash := sha256.Sum256(encoded)
	var command db.ConnectorCommand
	err = retryStateTx(ctx, t.Store, func(q *db.Queries) error {
		existing, e := q.GetConnectorCommandByID(ctx, id)
		if e == nil {
			if existing.EnterpriseID != identity.EnterpriseID || string(existing.PayloadHash) != string(hash[:]) || existing.CommandType != kind {
				return Error("REMOTE_COMMAND_CONFLICT")
			}
			command = existing
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = guardDispatch(ctx, q, identity, payload); e != nil {
			return e
		}
		connector, e := q.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID.UUID, EnterpriseID: identity.EnterpriseID})
		if e != nil || connector.Status != "online" {
			return Error("REMOTE_ROUTE_UNAVAILABLE")
		}
		if !slices.Contains(connector.Capabilities, "remote_execution.v1") {
			return Error("REMOTE_CONNECTOR_UPGRADE_REQUIRED")
		}
		var leaseID uuid.NullUUID
		needsCredential := false
		if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok {
			needsCredential = control.Operation == "open" || control.Operation == "reclaim" || control.Operation == "recover_files"
		}
		if identity.CredentialID.Valid && needsCredential {
			protocol := "ssh"
			if identity.Platform == "kubernetes" {
				protocol = "kubernetes"
			}
			lease, e := t.Secrets.PrepareLeaseWithQueries(secret.WithActorType(ctx, "connector"), q, connector.ID.String(), identity.EnterpriseID, secret.LeaseRequest{CredentialID: identity.CredentialID.UUID, OperationRef: commandID, TargetResourceType: identity.ResourceType, TargetResourceID: identity.ResourceID, RecipientType: "connector", RecipientID: connector.ID.String(), Protocol: protocol, TTL: time.Minute})
			if e != nil {
				return e
			}
			leaseID = uuid.NullUUID{UUID: lease.ID, Valid: true}
		}
		command, e = q.CreateConnectorCommand(ctx, db.CreateConnectorCommandParams{ID: id, CommandID: commandID, EnterpriseID: identity.EnterpriseID, ConnectorID: connector.ID, ConnectionEpoch: connector.ConnectionEpoch, OperationRef: commandID, CredentialLeaseID: leaseID, CommandType: kind, PayloadSchemaVersion: SchemaVersion, Payload: encoded, PayloadHash: hash[:], IdempotencyKey: key, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(timeout), Valid: true}})
		if e != nil {
			return e
		}
		if queued != nil {
			return queued(ctx, q, commandID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if t.Notifier != nil {
		t.Notifier.NotifyConnectorCommand(ctx, command.ConnectorID, command.ConnectionEpoch)
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		switch command.Status {
		case "succeeded":
			return command.Result, nil
		case "failed", "rejected", "cancelled", "expired":
			code := command.ErrorCode.String
			if code == "" {
				code = "REMOTE_COMMAND_FAILED"
			}
			if command.DispatchRejected || command.Status == "expired" {
				return command.Result, unsentCommandError{Error(code)}
			}
			return command.Result, Error(code)
		case "delivery_unknown", "result_unknown", "timed_out":
			return command.Result, Error("REMOTE_RESULT_UNKNOWN")
		}
		select {
		case <-ctx.Done():
			return nil, Error("REMOTE_RESULT_UNKNOWN")
		case <-deadline.C:
			return nil, Error("REMOTE_RESULT_UNKNOWN")
		case <-tick.C:
			command, err = t.Store.Queries.GetConnectorCommandByID(ctx, id)
			if err != nil {
				return nil, err
			}
		}
	}
}
