package connector

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/connectorprotocol"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/common/v1"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/operationsecret"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/sshtarget"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetrybinding"
	"github.com/kakj-go/Argus/internal/tlsmaterial"
	"github.com/kakj-go/Argus/internal/trustbundle"
)

const (
	ProtocolVersion      = "argus.connector/v2"
	MaxMessageBytes      = 1 << 20
	MaxInflightCommands  = 16
	defaultDispatchEvery = 500 * time.Millisecond
)

type Gateway struct {
	connectorv1.UnimplementedConnectorControlServiceServer
	Service                              Service
	Credentials                          secret.Service
	HeartbeatInterval                    time.Duration
	DispatchInterval                     time.Duration
	Dispatch                             *DispatchHub
	RemoteAccess                         *RemoteAccessHub
	Drain                                <-chan struct{}
	CreateCollectorEnrollment            func(context.Context, uuid.UUID) (CollectorEnrollmentMaterial, error)
	TelemetryTunnelLimit                 int
	TelemetryTunnelForwardTarget         string
	TelemetryTunnelIdentityForwardTarget string
	OperationSecretKey                   []byte
}

type CollectorEnrollmentMaterial struct {
	Token, EnrollmentEndpoint, IngestGRPCEndpoint, IngestHTTPEndpoint string
}

type activeCommand struct {
	acknowledged bool
	expiresAt    time.Time
}

func (gateway Gateway) Connect(stream connectorv1.ConnectorControlService_ConnectServer) error {
	identity, connectorRecord, err := gateway.trustedIdentity(stream.Context())
	if err != nil {
		return err
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := first.GetHello()
	if first.GetSequence() != 1 || hello == nil || hello.GetProtocolVersion() != ProtocolVersion || hello.GetInstanceId() != connectorRecord.InstanceID || len(hello.GetClientNonce()) < 16 || len(hello.GetClientNonce()) > 64 {
		return gateway.close(stream, 1, commonv1.CloseReason_CLOSE_REASON_PROTOCOL_ERROR, "CONNECTOR_PROTOCOL_ERROR")
	}
	opened, err := gateway.Service.OpenSession(stream.Context(), identity, hello.GetCapabilities())
	if err != nil {
		return gateway.close(stream, 1, commonv1.CloseReason_CLOSE_REASON_AUTHORIZATION_REVOKED, "CONNECTOR_FENCED")
	}
	epoch := opened.ConnectionEpoch
	defer gateway.Service.Disconnect(context.Background(), identity, epoch)
	nodeKind := "connector"
	if connectorRecord.Role == "kubernetes" {
		nodeKind = "kubernetes_connector"
	}
	trustNode := trustbundle.Node{Kind: nodeKind, ID: identity.ConnectorID.String(),
		EnterpriseID: uuid.NullUUID{UUID: identity.EnterpriseID, Valid: true}}
	bundle, trustCurrent, err := gateway.Service.TrustBundles.Observe(stream.Context(), trustNode, trustbundle.Acknowledgement{
		Epoch: int64(hello.GetTrustBundleEpoch()), SHA256: hello.GetTrustBundleSha256(), Fingerprints: hello.GetTrustBundleCaFingerprints(),
	})
	if err != nil {
		return gateway.close(stream, 1, commonv1.CloseReason_CLOSE_REASON_PROTOCOL_ERROR, "TRUST_BUNDLE_STATE_INVALID")
	}
	serverNonce := make([]byte, 32)
	if _, err := rand.Read(serverNonce); err != nil {
		return err
	}
	heartbeat := gateway.heartbeatInterval()
	serverSequence := uint64(1)
	if err := stream.Send(&connectorv1.ConnectResponse{Sequence: serverSequence, Frame: &connectorv1.ConnectResponse_Welcome{Welcome: &connectorv1.ConnectorWelcome{
		ProtocolVersion: ProtocolVersion, ConnectionEpoch: uint64(epoch), ServerTime: timestamppb.Now(), HeartbeatInterval: durationpb.New(heartbeat),
		MaxMessageBytes: MaxMessageBytes, MaxInflightCommands: MaxInflightCommands, ServerNonce: serverNonce,
		CertificateRotationRequested: opened.CertificateRotationRequestedAt.Valid}}}); err != nil {
		return err
	}
	serverSequence++
	if !trustCurrent {
		if err := stream.Send(&connectorv1.ConnectResponse{Sequence: serverSequence, Frame: &connectorv1.ConnectResponse_TrustBundleUpdate{
			TrustBundleUpdate: trustBundleUpdate(bundle)}}); err != nil {
			return err
		}
		serverSequence++
	}
	if err := gateway.sendReconcile(stream, identity, epoch, &serverSequence); err != nil {
		return err
	}
	tunnelLeases := make(map[uuid.UUID]cachedConnectorTunnelLease)
	defer gateway.releaseTelemetryTunnels(identity, epoch, tunnelLeases)
	if err := gateway.sendTelemetryTunnelDesired(stream, identity, epoch, &serverSequence, tunnelLeases); err != nil {
		return err
	}

	received := make(chan receiveResult, 1)
	remoteOutbound := make(chan *connectorv1.ConnectResponse, 32)
	unregisterRemote := func() {}
	if gateway.RemoteAccess != nil {
		unregisterRemote = gateway.RemoteAccess.Register(identity.ConnectorID, epoch, remoteOutbound)
	}
	defer unregisterRemote()
	go receiveConnectorFrames(stream, received)
	dispatchTicker := time.NewTicker(gateway.dispatchInterval())
	defer dispatchTicker.Stop()
	heartbeatDeadline := time.NewTimer(3 * heartbeat)
	defer heartbeatDeadline.Stop()
	lastClientSequence := uint64(1)
	pendingAcknowledgements := map[uint64]string{}
	activeCommands := map[string]activeCommand{}
	dispatchWakeup, unregisterDispatch := gateway.Dispatch.Register(identity.ConnectorID)
	defer unregisterDispatch()
	defer gateway.markUncertain(identity, epoch, activeCommands)
	dispatchCommands := func() error {
		removeExpiredActiveCommands(time.Now(), activeCommands, pendingAcknowledgements)
		available := MaxInflightCommands - int32(len(activeCommands))
		if available <= 0 {
			return nil
		}
		commands, err := gateway.Service.ListCommands(stream.Context(), identity, epoch, available)
		if err != nil {
			return err
		}
		for _, command := range commands {
			ready, failureCode, gateErr := gateway.collectorTunnelDispatchGate(stream.Context(), command)
			if gateErr != nil {
				return gateErr
			}
			if failureCode != "" {
				_, _ = gateway.Service.TransitionCommand(stream.Context(), identity, epoch, command.CommandID, "failed", nil, failureCode)
				continue
			}
			if !ready {
				continue
			}
			frame, err := gateway.typedCommand(stream.Context(), command)
			if err != nil {
				_, _ = gateway.Service.TransitionCommand(stream.Context(), identity, epoch, command.CommandID, "failed", nil, "CONNECTOR_COMMAND_SCHEMA_INVALID")
				continue
			}
			transitioned, err := gateway.Service.TransitionCommand(stream.Context(), identity, epoch, command.CommandID, "dispatched", nil, "")
			if err != nil || cancelledHostInstallCommand(transitioned) {
				continue
			}
			if err := stream.Send(&connectorv1.ConnectResponse{Sequence: serverSequence, Frame: &connectorv1.ConnectResponse_Command{Command: frame}}); err != nil {
				_, _ = gateway.Service.TransitionCommand(context.Background(), identity, epoch, command.CommandID, "delivery_unknown", nil, "CONNECTOR_DISCONNECTED")
				delete(activeCommands, command.CommandID)
				return err
			}
			pendingAcknowledgements[serverSequence] = command.CommandID
			activeCommands[command.CommandID] = activeCommand{expiresAt: command.ExpiresAt.Time}
			serverSequence++
		}
		return nil
	}
	for {
		select {
		case <-gateway.Drain:
			return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_SERVER_DRAIN, "CONNECTOR_GATEWAY_DRAINING")
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-heartbeatDeadline.C:
			return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_SESSION_EXPIRED, "CONNECTOR_HEARTBEAT_TIMEOUT")
		case item := <-received:
			if item.err != nil {
				if errors.Is(item.err, io.EOF) {
					return nil
				}
				return item.err
			}
			message := item.message
			if message.GetSequence() != lastClientSequence+1 || message.GetHello() != nil || message.GetRemoteAccessData() != nil {
				return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_PROTOCOL_ERROR, "CONNECTOR_SEQUENCE_INVALID")
			}
			lastClientSequence = message.GetSequence()
			if leaseRequest := message.GetCredentialLeaseRequest(); leaseRequest != nil {
				grant, err := gateway.fulfillCredentialLease(stream.Context(), identity, epoch, leaseRequest)
				if err != nil {
					return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_AUTHORIZATION_REVOKED, "CREDENTIAL_LEASE_INVALID")
				}
				if grant != nil {
					if err := stream.Send(&connectorv1.ConnectResponse{Sequence: serverSequence, Frame: &connectorv1.ConnectResponse_CredentialLeaseGrant{CredentialLeaseGrant: grant}}); err != nil {
						return err
					}
					// grpc-go may retain the message after Send returns. The Connector
					// owns zeroing the received copy after the command has completed.
					serverSequence++
				} else {
					delete(activeCommands, leaseRequest.GetCommandId())
				}
			}
			if rotationRequest := message.GetCertificateRotationRequest(); rotationRequest != nil {
				if rotationRequest.GetConnectionEpoch() != uint64(epoch) {
					return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_AUTHORIZATION_REVOKED, "CONNECTOR_FENCED")
				}
				certificate, err := gateway.Service.RotateCertificate(stream.Context(), identity, epoch, rotationRequest.GetCsrPem())
				if err != nil {
					return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_AUTHORIZATION_REVOKED, "CONNECTOR_CERTIFICATE_ROTATION_FAILED")
				}
				grant := &connectorv1.CertificateRotationGrant{ConnectionEpoch: uint64(epoch), CertificatePem: []byte(certificate.PEM),
					NotAfter: timestamppb.New(certificate.NotAfter)}
				if err := stream.Send(&connectorv1.ConnectResponse{Sequence: serverSequence, Frame: &connectorv1.ConnectResponse_CertificateRotationGrant{
					CertificateRotationGrant: grant}}); err != nil {
					return err
				}
				serverSequence++
			}
			if err := gateway.handleFrame(stream.Context(), identity, epoch, message, pendingAcknowledgements, activeCommands); err != nil {
				return gateway.close(stream, serverSequence, commonv1.CloseReason_CLOSE_REASON_PROTOCOL_ERROR, "CONNECTOR_FRAME_INVALID")
			}
			if message.GetHeartbeat() != nil || message.GetTelemetryTunnelStatusSet() != nil {
				if err := gateway.sendTelemetryTunnelDesired(stream, identity, epoch, &serverSequence, tunnelLeases); err != nil {
					return err
				}
			}
			if message.GetHeartbeat() != nil {
				if !heartbeatDeadline.Stop() {
					select {
					case <-heartbeatDeadline.C:
					default:
					}
				}
				heartbeatDeadline.Reset(3 * heartbeat)
			}
			if err := stream.Send(&connectorv1.ConnectResponse{Sequence: serverSequence, Frame: &connectorv1.ConnectResponse_Acknowledge{
				Acknowledge: &connectorv1.ServerAcknowledge{ClientSequence: lastClientSequence}}}); err != nil {
				return err
			}
			serverSequence++
		case <-dispatchTicker.C:
			if err := dispatchCommands(); err != nil {
				return err
			}
		case <-dispatchWakeup:
			if err := dispatchCommands(); err != nil {
				return err
			}
		case frame := <-remoteOutbound:
			frame.Sequence = serverSequence
			if err := stream.Send(frame); err != nil {
				return err
			}
			serverSequence++
		}
	}
}

func (gateway Gateway) collectorTunnelDispatchGate(ctx context.Context, command db.ConnectorCommand) (bool, string, error) {
	if command.CommandType != "collector_management" {
		return true, "", nil
	}
	var request connectorv1.CollectorManagementCommand
	if protojson.Unmarshal(command.Payload, &request) != nil {
		return false, "CONNECTOR_COMMAND_SCHEMA_INVALID", nil
	}
	transport := request.GetTransport()
	if request.GetOperation() == "uninstall" || (transport != "executor_tunnel" && transport != "bastion_tunnel") {
		return true, "", nil
	}
	collectorID, err := uuid.Parse(request.GetCollectorId())
	if err != nil {
		return false, "CONNECTOR_COMMAND_SCHEMA_INVALID", nil
	}
	tunnel, err := gateway.Service.Store.Queries.GetTelemetryTunnelByCollector(ctx, collectorID)
	if err != nil {
		return false, "", err
	}
	if tunnel.EnterpriseID != command.EnterpriseID || tunnel.Transport != transport {
		return false, "COLLECTOR_ROUTE_INVALID", nil
	}
	if tunnel.Status == "established" {
		return true, "", nil
	}
	switch tunnel.LastDropReason {
	case "loopback_port_conflict":
		return false, "COLLECTOR_LOOPBACK_PORT_CONFLICT", nil
	case "tunnel_forward_target_unconfigured":
		return false, "TUNNEL_FORWARD_TARGET_UNCONFIGURED", nil
	case "tunnel_quota_exceeded":
		return false, "TUNNEL_QUOTA_EXCEEDED", nil
	case "credential_revoked", "credential_unavailable":
		return false, "CREDENTIAL_UNAVAILABLE", nil
	case "host_key_changed":
		return false, "COLLECTOR_TARGET_HOST_KEY_CHANGED", nil
	}
	if tunnel.Status == "removed" {
		return false, "COLLECTOR_MANAGEMENT_FAILED", nil
	}
	return false, "", nil
}

type receiveResult struct {
	message *connectorv1.ConnectRequest
	err     error
}

func receiveConnectorFrames(stream connectorv1.ConnectorControlService_ConnectServer, output chan<- receiveResult) {
	for {
		message, err := stream.Recv()
		output <- receiveResult{message: message, err: err}
		if err != nil {
			return
		}
	}
}

func (gateway Gateway) handleFrame(ctx context.Context, identity TrustedIdentity, epoch int64, request *connectorv1.ConnectRequest, acknowledgements map[uint64]string, active map[string]activeCommand) error {
	switch {
	case request.GetHeartbeat() != nil:
		if request.GetHeartbeat().GetConnectionEpoch() != uint64(epoch) {
			return ErrConnectorFenced
		}
		if err := gateway.Service.UpdateHostRuntimeObservation(ctx, identity, request.GetHeartbeat().GetHostRuntime()); err != nil {
			return err
		}
		if err := gateway.Service.UpdateBastionRelayStatus(ctx, identity, request.GetHeartbeat().GetBastionRelay()); err != nil {
			return err
		}
		if err := gateway.applyTelemetryTunnelStatuses(ctx, identity, epoch, request.GetHeartbeat().GetTelemetryTunnels()); err != nil {
			return err
		}
		return gateway.Service.Heartbeat(ctx, identity, epoch)
	case request.GetTelemetryTunnelStatusSet() != nil:
		return gateway.applyTelemetryTunnelStatuses(ctx, identity, epoch, request.GetTelemetryTunnelStatusSet().GetTunnels())
	case request.GetAcknowledge() != nil:
		sequence := request.GetAcknowledge().GetServerSequence()
		commandID, ok := acknowledgements[sequence]
		if !ok {
			return ErrCommandState
		}
		transitioned, err := gateway.Service.TransitionCommand(ctx, identity, epoch, commandID, "acknowledged", nil, "")
		if err != nil {
			return err
		}
		delete(acknowledgements, sequence)
		if cancelledHostInstallCommand(transitioned) {
			delete(active, commandID)
			return nil
		}
		command := active[commandID]
		command.acknowledged = true
		active[commandID] = command
		return nil
	case request.GetCommandResult() != nil:
		result := request.GetCommandResult()
		if result.GetConnectionEpoch() != uint64(epoch) || len(result.GetResult()) != 0 || result.GetTypedResult() == nil || result.GetResultSchemaVersion() == "" {
			return ErrCommandState
		}
		command, err := gateway.Service.Store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: result.GetCommandId(), ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
		if err != nil || !resultTypeAllowed(command.CommandType, result.GetTypedResult()) {
			return ErrCommandState
		}
		if cancelledHostInstallCommand(command) {
			delete(active, result.GetCommandId())
			return nil
		}
		value, err := marshalTypedResult(result.GetTypedResult())
		if err != nil || len(value) > MaxMessageBytes {
			return ErrCommandState
		}
		errorCode := ""
		if result.GetError() != nil {
			errorCode = result.GetError().GetCode()
		}
		next := normalizeResultStatus(result.GetStatus())
		if command.CommandType == "host_connection_probe" {
			_, next, errorCode, err = hostProbeOutcome(command.Payload, next, result.GetTypedResult(), errorCode)
			if err != nil {
				return err
			}
		}
		if command.CommandType == "connector_uninstall" && next == "succeeded" && !validConnectorUninstallResult(result.GetTypedResult()) {
			return ErrCommandState
		}
		if command.CommandType == "collector_management" && next == "succeeded" && !validCollectorManagementResult(command, result.GetTypedResult()) {
			return ErrCommandState
		}
		transitioned, transitionErr := gateway.Service.TransitionCommand(ctx, identity, epoch, result.GetCommandId(), next, value, errorCode)
		if transitionErr != nil {
			return transitionErr
		}
		if cancelledHostInstallCommand(transitioned) {
			delete(active, result.GetCommandId())
			return nil
		}
		if next != "running" {
			delete(active, result.GetCommandId())
		}
		return gateway.completeConnectionTest(ctx, command, next, result.GetTypedResult(), errorCode)
	case request.GetCommandReconcileResult() != nil:
		result := request.GetCommandReconcileResult()
		if result.GetConnectionEpoch() != uint64(epoch) {
			return ErrConnectorFenced
		}
		for _, item := range result.GetCommands() {
			next := normalizeResultStatus(item.GetStatus())
			if next != "succeeded" && next != "failed" {
				continue
			}
			errorCode := ""
			if item.GetError() != nil {
				errorCode = item.GetError().GetCode()
			}
			command, err := gateway.Service.Store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: item.GetCommandId(), ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
			if err != nil {
				continue
			}
			next = reconciledCommandStatus(command.CommandType, next)
			command, err = gateway.Service.TransitionCommand(ctx, identity, epoch, item.GetCommandId(), next, nil, errorCode)
			if err == nil && (command.CommandType == "host_connection_probe" || command.CommandType == "kubernetes_connection_probe") {
				_ = gateway.completeConnectionTest(ctx, command, "result_unknown", nil, "CONNECTOR_RESULT_NOT_REPLAYABLE")
			}
		}
		return nil
	case request.GetCredentialLeaseRequest() != nil:
		return nil
	case request.GetRemoteAccessOutput() != nil || request.GetRemoteAccessState() != nil || request.GetRemoteAccessClose() != nil:
		if gateway.RemoteAccess == nil {
			return ErrCommandState
		}
		return gateway.RemoteAccess.Deliver(identity.ConnectorID, epoch, request)
	case request.GetCertificateRotationRequest() != nil:
		return nil
	case request.GetTrustBundleAcknowledge() != nil:
		acknowledgement := request.GetTrustBundleAcknowledge()
		nodeKind := "connector"
		current, err := gateway.Service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
		if err != nil {
			return err
		}
		if current.Role == "kubernetes" {
			nodeKind = "kubernetes_connector"
		}
		return gateway.Service.TrustBundles.Acknowledge(ctx, trustbundle.Node{Kind: nodeKind, ID: identity.ConnectorID.String(),
			EnterpriseID: uuid.NullUUID{UUID: identity.EnterpriseID, Valid: true}}, trustbundle.Acknowledgement{
			Epoch: int64(acknowledgement.GetEpoch()), SHA256: acknowledgement.GetBundleSha256(), Fingerprints: acknowledgement.GetCaFingerprints(),
		})
	default:
		return ErrCommandState
	}
}

func trustBundleUpdate(bundle trustbundle.Bundle) *connectorv1.TrustBundleUpdate {
	value := &connectorv1.TrustBundleUpdate{Epoch: uint64(bundle.Epoch), BundlePem: bundle.Material.PEM,
		BundleSha256: bundle.Material.SHA256, CurrentCaFingerprints: bundle.CurrentCAFingerprints,
		NextCaFingerprints: bundle.NextCAFingerprints, State: bundle.State, StartedAt: timestamppb.New(bundle.StartedAt)}
	if !bundle.RetireAt.IsZero() {
		value.RetireAt = timestamppb.New(bundle.RetireAt)
	}
	return value
}

func marshalTypedResult(value *anypb.Any) ([]byte, error) {
	if value == nil {
		return nil, ErrCommandState
	}
	message, err := value.UnmarshalNew()
	if err != nil {
		return nil, err
	}
	return protojson.MarshalOptions{UseProtoNames: true}.Marshal(message)
}

func (gateway Gateway) fulfillCredentialLease(ctx context.Context, identity TrustedIdentity, epoch int64, request *connectorv1.CredentialLeaseRequest) (*connectorv1.CredentialLeaseGrant, error) {
	if gateway.Credentials.Store == nil || request.GetConnectionEpoch() != uint64(epoch) || len(request.GetRecipientNonce()) < 16 || len(request.GetRecipientNonce()) > 64 {
		return nil, secret.ErrInvalidLease
	}
	leaseID, err := uuid.Parse(request.GetLeaseId())
	if err != nil {
		return nil, secret.ErrInvalidLease
	}
	command, err := gateway.Service.Store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: request.GetCommandId(), ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
	commandID := request.GetCommandId()
	if err == nil {
		if cancelledHostInstallCommand(command) && command.CredentialLeaseID.Valid && command.CredentialLeaseID.UUID == leaseID {
			return nil, nil
		}
		if !command.CredentialLeaseID.Valid || command.CredentialLeaseID.UUID != leaseID || (command.Status != "acknowledged" && command.Status != "running") {
			return nil, secret.ErrInvalidLease
		}
	} else {
		lease, leaseErr := gateway.Service.Store.Queries.GetCredentialLease(ctx, db.GetCredentialLeaseParams{ID: leaseID, EnterpriseID: identity.EnterpriseID})
		if leaseErr != nil || lease.OperationRef != commandID ||
			lease.RecipientType != "connector" || lease.RecipientID != identity.ConnectorID.String() || lease.Status != "active" {
			return nil, secret.ErrInvalidLease
		}
		switch lease.TargetResourceType {
		case "remote_access_session":
		case "telemetry_tunnel":
			tunnel, tunnelErr := gateway.Service.Store.Queries.GetTelemetryTunnel(ctx, db.GetTelemetryTunnelParams{
				ID: lease.TargetResourceID, EnterpriseID: identity.EnterpriseID})
			if tunnelErr != nil || !tunnel.ConnectorID.Valid || tunnel.ConnectorID.UUID != identity.ConnectorID ||
				tunnel.LeaseOwner != connectorTunnelOwner(identity.ConnectorID, epoch) ||
				tunnel.OwnerConnectionEpoch != epoch || tunnel.Status == "removed" ||
				lease.OperationRef != fmt.Sprintf("telemetry_tunnel:%s:%d:%d", tunnel.ID, tunnel.Epoch, tunnel.Fence) {
				return nil, secret.ErrInvalidLease
			}
		default:
			return nil, secret.ErrInvalidLease
		}
	}
	issued, err := gateway.Credentials.FulfillLease(ctx, identity.EnterpriseID, leaseID, "connector", identity.ConnectorID.String())
	if err != nil {
		return nil, err
	}
	var operationPayload []byte
	if request.GetHostOnboardingOperationId() != "" {
		operationID, parseErr := uuid.Parse(request.GetHostOnboardingOperationId())
		if parseErr != nil || len(gateway.OperationSecretKey) != 32 {
			return nil, secret.ErrInvalidLease
		}
		operation, getErr := gateway.Service.Store.Queries.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: operationID, EnterpriseID: identity.EnterpriseID})
		if getErr != nil || operation.Status != "running" || operation.SshPath != "bastion_connector" || !operation.BastionScopeID.Valid {
			return nil, secret.ErrInvalidLease
		}
		scope, scopeErr := gateway.Service.Store.Queries.GetBastionScope(ctx, db.GetBastionScopeParams{ID: operation.BastionScopeID.UUID, EnterpriseID: identity.EnterpriseID})
		if scopeErr != nil || !scope.ActiveConnectorID.Valid || scope.ActiveConnectorID.UUID != identity.ConnectorID {
			return nil, secret.ErrInvalidLease
		}
		record, secretErr := gateway.Service.Store.Queries.GetHostOnboardingOperationSecret(ctx, db.GetHostOnboardingOperationSecretParams{OperationID: operation.ID, EnterpriseID: identity.EnterpriseID})
		if secretErr != nil {
			return nil, secret.ErrInvalidLease
		}
		material, decryptErr := operationsecret.Decrypt(gateway.OperationSecretKey, record.Nonce, record.Ciphertext, identity.EnterpriseID, operation.ID)
		if decryptErr != nil {
			return nil, secret.ErrInvalidLease
		}
		operationPayload = []byte(material.EnrollmentToken)
	}
	return &connectorv1.CredentialLeaseGrant{LeaseId: leaseID.String(), CommandId: commandID, ConnectionEpoch: uint64(epoch),
		CredentialPayload: issued.Value, OperationSecretPayload: operationPayload, ExpiresAt: timestamppb.New(issued.Lease.ExpiresAt.Time), RecipientNonce: request.GetRecipientNonce()}, nil
}

func (gateway Gateway) completeConnectionTest(ctx context.Context, command db.ConnectorCommand, status string, typed *anypb.Any, errorCode string) error {
	if command.CommandType != "host_connection_probe" && command.CommandType != "kubernetes_connection_probe" {
		return nil
	}
	testID, err := uuid.Parse(command.OperationRef)
	if err != nil {
		return ErrCommandState
	}
	result := resource.ConnectionTestResult{}
	if command.CommandType == "host_connection_probe" {
		result, status, errorCode, err = hostProbeOutcome(command.Payload, status, typed, errorCode)
		if err != nil {
			return err
		}
	} else if status == "succeeded" {
		var value connectorv1.KubernetesConnectionProbeResult
		if typed.UnmarshalTo(&value) != nil {
			return ErrCommandState
		}
		result.RemoteVersion = value.ServerVersion
	}
	encoded, _ := json.Marshal(result)
	_, err = gateway.Service.Store.Queries.CompleteConnectionTest(ctx, db.CompleteConnectionTestParams{ID: testID, EnterpriseID: command.EnterpriseID,
		Status: status, Result: encoded, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""}})
	return err
}

func hostProbeConnectionTestResult(value *connectorv1.HostConnectionProbeResult) resource.ConnectionTestResult {
	result := resource.ConnectionTestResult{
		ResolvedIPs: value.GetResolvedIps(), HostKeyFingerprint: value.GetHostKeyFingerprint(),
		RemoteVersion: value.GetRemoteVersion(), LatencyMS: int64(value.GetLatencyMillis()),
		Platform: value.GetPlatform(), Architecture: value.GetArchitecture(), DistributionVersion: value.GetDistributionVersion(),
		ServiceManager: value.GetServiceManager(), Privileged: value.GetPrivileged(), FreeDiskBytes: value.GetFreeDiskBytes(),
		CallbackVerified: value.GetCallbackVerified(), CallbackControlPath: value.GetCallbackControlPath(),
		Checks: sshtarget.ConnectionChecks(sshtarget.Evidence{Platform: value.GetPlatform(), Architecture: value.GetArchitecture(),
			DistributionVersion: value.GetDistributionVersion(), ServiceManager: value.GetServiceManager(), Privileged: value.GetPrivileged(), FreeDiskBytes: value.GetFreeDiskBytes()}),
	}
	if result.CallbackVerified {
		result.Checks = append(result.Checks, map[string]string{"name": "onboarding_callback", "status": "passed", "detail": result.CallbackControlPath})
	}
	return result
}

func (gateway Gateway) trustedIdentity(ctx context.Context) (TrustedIdentity, db.Connector, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return TrustedIdentity{}, db.Connector{}, ErrConnectorFenced
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) != 1 {
		return TrustedIdentity{}, db.Connector{}, ErrConnectorFenced
	}
	certificate := tlsInfo.State.PeerCertificates[0]
	connectorID, err := connectorIDFromCertificate(certificate)
	if err != nil {
		return TrustedIdentity{}, db.Connector{}, err
	}
	value, err := gateway.Service.Store.Queries.GetConnectorByID(ctx, connectorID)
	if err != nil {
		return TrustedIdentity{}, db.Connector{}, ErrConnectorFenced
	}
	pkiIdentity, err := gateway.Service.Store.Queries.GetActivePKICertificateIdentity(ctx, trustbundle.CertificateSerial(certificate))
	if err != nil || trustbundle.VerifyCertificateIdentity(pkiIdentity, certificate, trustbundle.CertificateIdentity{Kind: "connector",
		SubjectID: connectorID.String(), EnterpriseID: uuid.NullUUID{UUID: value.EnterpriseID, Valid: true}, Usage: x509.ExtKeyUsageClientAuth}) != nil {
		return TrustedIdentity{}, db.Connector{}, ErrConnectorFenced
	}
	identity := TrustedIdentity{ConnectorID: connectorID, EnterpriseID: value.EnterpriseID, SerialNumber: certificate.SerialNumber.String()}
	return identity, value, nil
}

func connectorIDFromCertificate(certificate *x509.Certificate) (uuid.UUID, error) {
	if certificate == nil || len(certificate.URIs) != 1 || certificate.URIs[0].Scheme != "spiffe" || certificate.URIs[0].Host != "argus.io" {
		return uuid.Nil, ErrConnectorFenced
	}
	parts := strings.Split(strings.Trim(certificate.URIs[0].Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "connector" {
		return uuid.Nil, ErrConnectorFenced
	}
	value, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, ErrConnectorFenced
	}
	return value, nil
}

func typedCommand(command db.ConnectorCommand) (*connectorv1.ConnectorCommand, error) {
	return (Gateway{}).typedCommand(context.Background(), command)
}

func (gateway Gateway) typedCommand(ctx context.Context, command db.ConnectorCommand) (*connectorv1.ConnectorCommand, error) {
	var payload proto.Message
	switch command.CommandType {
	case "host_connection_probe":
		var plan struct {
			Address    string                          `json:"address"`
			Port       uint32                          `json:"port"`
			Platform   string                          `json:"platform"`
			Username   string                          `json:"username"`
			Onboarding *installation.CallbackProbePlan `json:"onboarding"`
		}
		if err := json.Unmarshal(command.Payload, &plan); err != nil || plan.Address == "" || plan.Port == 0 {
			return nil, ErrCommandState
		}
		if plan.Platform != "linux" && plan.Platform != "windows" {
			return nil, ErrCommandState
		}
		probe := &connectorv1.HostConnectionProbe{Address: plan.Address, Port: plan.Port, Protocol: "ssh", Username: plan.Username, Platform: plan.Platform}
		if p := plan.Onboarding; p != nil {
			probe.Onboarding = &connectorv1.CallbackProbePlan{ControlPath: p.ControlPath, EnrollmentEndpoint: p.EnrollmentEndpoint, GatewayEndpoint: p.GatewayEndpoint,
				EnrollDialAddress: p.EnrollDialAddress, GatewayDialAddress: p.GatewayDialAddress, TrustBundlePem: p.TrustBundlePEM, TrustBundleEpoch: p.TrustBundleEpoch, RelayPortGeneration: p.RelayPortGeneration}
		}
		payload = probe
	case "host_connector_install":
		var plan installation.HostConnectorInstallPlan
		if json.Unmarshal(command.Payload, &plan) != nil || plan.ConnectorID == uuid.Nil || !plan.TargetPlatform.Valid() {
			return nil, ErrCommandState
		}
		payload = &connectorv1.HostConnectorInstall{OperationId: command.OperationRef, HostId: plan.HostID.String(), ConnectorId: plan.ConnectorID.String(),
			TargetPlatform: string(plan.TargetPlatform), TargetDistributionVersion: plan.DistributionVersion, Address: plan.Address, Port: uint32(plan.Port), Username: plan.Username,
			PinnedHostKey: plan.PinnedHostKey, ReleaseVersionId: plan.ReleaseVersionID.String(), ManifestUri: plan.ManifestURI,
			Artifact: &connectorv1.CollectorArtifact{Platform: plan.Artifact.Platform, Uri: plan.Artifact.URI, Sha256: plan.Artifact.SHA256,
				Signature: plan.Artifact.Signature, SigningKeyId: plan.Artifact.SigningKeyID, ByteSize: uint64(plan.Artifact.ByteSize)},
			SigningPublicKey: plan.SigningPublicKey, EnrollmentEndpoint: plan.EnrollmentEndpoint, GatewayEndpoint: plan.GatewayEndpoint,
			EnrollDialAddress: plan.EnrollDialAddress, GatewayDialAddress: plan.GatewayDialAddress, TrustBundlePem: plan.TrustBundlePEM,
			TrustBundleEpoch: uint64(plan.TrustBundleEpoch), TrustBundleSha256: plan.TrustBundleSHA256}
	case "host_connector_removal":
		var plan hostremoval.Plan
		if json.Unmarshal(command.Payload, &plan) != nil || !plan.OperationID.Valid || plan.OperationID.UUID.String() != command.OperationRef ||
			plan.HostID == uuid.Nil || plan.ConnectorID == uuid.Nil || plan.RemovalGeneration < 1 || plan.Address == "" || plan.Port < 1 || plan.Username == "" || plan.PinnedHostKey == "" {
			return nil, ErrCommandState
		}
		payload = &connectorv1.HostConnectorRemoval{OperationId: plan.OperationID.UUID.String(), HostId: plan.HostID.String(), ConnectorId: plan.ConnectorID.String(),
			RemovalGeneration: uint64(plan.RemovalGeneration), TargetPlatform: plan.TargetPlatform, Address: plan.Address, Port: uint32(plan.Port),
			Username: plan.Username, PinnedHostKey: plan.PinnedHostKey,
			ManagedChangeBeforeJson: plan.ManagedChangeBefore, ManagedChangeAppliedJson: plan.ManagedChangeApplied}
		if plan.ManagedChangeID.Valid {
			payload.(*connectorv1.HostConnectorRemoval).ManagedChangeId = plan.ManagedChangeID.UUID.String()
		}
	case "host_windows_rdp_configure":
		var request connectorv1.HostWindowsRDPConfigure
		if json.Unmarshal(command.Payload, &request) != nil || request.GetHostId() == "" || !request.GetEnable() || !request.GetEnforceNla() || !request.GetEnableFirewall() {
			return nil, ErrCommandState
		}
		payload = &request
	case "kubernetes_connection_probe":
		var plan struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(command.Payload, &plan); err != nil || plan.Address == "" {
			return nil, ErrCommandState
		}
		payload = &connectorv1.KubernetesConnectionProbe{ApiServer: plan.Address}
	case "kubernetes_resource_query":
		var query connectorv1.KubernetesResourceQuery
		if err := json.Unmarshal(command.Payload, &query); err != nil || query.ClusterId == "" || query.ResourceType == "" {
			return nil, ErrCommandState
		}
		payload = &query
	case "kubernetes_pod_logs":
		var query connectorv1.KubernetesPodLogsQuery
		if err := json.Unmarshal(command.Payload, &query); err != nil || query.ClusterId == "" || query.Namespace == "" || query.Pod == "" {
			return nil, ErrCommandState
		}
		payload = &query
	case "connector_uninstall":
		var request connectorv1.ConnectorUninstall
		if err := json.Unmarshal(command.Payload, &request); err != nil || request.ConnectorId == "" || request.ExpectedConnectionEpoch == 0 {
			return nil, ErrCommandState
		}
		payload = &request
	case "collector_management":
		var request connectorv1.CollectorManagementCommand
		if protojson.Unmarshal(command.Payload, &request) != nil || request.GetCollectorId() == "" || request.GetResourceId() == "" || request.GetArtifact() == nil {
			return nil, ErrCommandState
		}
		if request.GetOperation() == "install" && gateway.CreateCollectorEnrollment != nil {
			collectorID, parseErr := uuid.Parse(request.GetCollectorId())
			if parseErr != nil {
				return nil, ErrCommandState
			}
			material, materialErr := gateway.CreateCollectorEnrollment(ctx, collectorID)
			if materialErr != nil {
				return nil, materialErr
			}
			request.EnrollmentToken = []byte(material.Token)
			request.EnrollmentEndpoint = material.EnrollmentEndpoint
			request.IngestGrpcEndpoint = material.IngestGRPCEndpoint
			request.IngestHttpEndpoint = material.IngestHTTPEndpoint
		}
		payload = &request
	default:
		return nil, ErrCommandState
	}
	typed, err := anypb.New(payload)
	if err != nil {
		return nil, err
	}
	frame := &connectorv1.ConnectorCommand{CommandId: command.CommandID, ConnectionEpoch: uint64(command.ConnectionEpoch), CommandType: command.CommandType,
		PayloadHash: hex.EncodeToString(command.PayloadHash), ExpiresAt: timestamppb.New(command.ExpiresAt.Time), IdempotencyKey: command.IdempotencyKey,
		TypedPayload: typed, PayloadSchemaVersion: command.PayloadSchemaVersion, OperationRef: command.OperationRef}
	if command.CredentialLeaseID.Valid {
		frame.CredentialLeaseId = command.CredentialLeaseID.UUID.String()
	}
	return frame, nil
}

func resultTypeAllowed(commandType string, value *anypb.Any) bool {
	return connectorprotocol.ResultAllowed(commandType, value)
}

func validConnectorUninstallResult(value *anypb.Any) bool {
	var result connectorv1.ConnectorUninstallResult
	return value != nil && value.UnmarshalTo(&result) == nil && result.GetIdentityRemoved() && result.GetServiceStopped()
}

func validCollectorManagementResult(command db.ConnectorCommand, value *anypb.Any) bool {
	var request connectorv1.CollectorManagementCommand
	var result connectorv1.CollectorManagementResult
	if protojson.Unmarshal(command.Payload, &request) != nil || value == nil || value.UnmarshalTo(&result) != nil {
		return false
	}
	if result.GetCollectorId() != request.GetCollectorId() || (result.GetStatus() != "converged" && result.GetStatus() != "uninstalled") {
		return false
	}
	if request.GetResourceType() == "kubernetes_cluster" && request.GetOperation() != "uninstall" {
		nodes := make([]telemetrybinding.NodeEvidence, 0, len(result.GetKubernetesNodes()))
		for _, node := range result.GetKubernetesNodes() {
			nodes = append(nodes, telemetrybinding.NodeEvidence{NodeUID: node.GetNodeUid(), NodeName: node.GetNodeName(),
				ProviderID: node.GetProviderId(), MachineID: node.GetMachineId(), SystemUUID: node.GetSystemUuid(), InternalIPs: node.GetInternalIps()})
		}
		if telemetrybinding.Validate(nodes) != nil {
			return false
		}
	} else if len(result.GetKubernetesNodes()) != 0 {
		return false
	}
	return request.GetOperation() == "uninstall" || (result.GetEffectiveRevision() == request.GetDesiredRevision() &&
		strings.EqualFold(result.GetAppliedConfigSha256(), request.GetConfigSha256()))
}

func reconciledCommandStatus(commandType, reported string) string {
	if (commandType == "connector_uninstall" || commandType == "collector_management") && reported == "succeeded" {
		return "result_unknown"
	}
	return reported
}

func (gateway Gateway) sendReconcile(stream connectorv1.ConnectorControlService_ConnectServer, identity TrustedIdentity, epoch int64, sequence *uint64) error {
	commands, err := gateway.Service.ListUncertainCommands(stream.Context(), identity, 64)
	if err != nil || len(commands) == 0 {
		return err
	}
	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		ids = append(ids, command.CommandID)
	}
	if err := stream.Send(&connectorv1.ConnectResponse{Sequence: *sequence, Frame: &connectorv1.ConnectResponse_CommandReconcileRequest{
		CommandReconcileRequest: &connectorv1.CommandReconcileRequest{CommandIds: ids, ConnectionEpoch: uint64(epoch)}}}); err != nil {
		return err
	}
	*sequence++
	return nil
}

func (gateway Gateway) sendTelemetryTunnelDesired(stream connectorv1.ConnectorControlService_ConnectServer,
	identity TrustedIdentity, epoch int64, sequence *uint64, cache map[uuid.UUID]cachedConnectorTunnelLease) error {
	desired, err := gateway.desiredTelemetryTunnels(stream.Context(), identity, epoch, cache)
	if err != nil {
		return err
	}
	if err = stream.Send(&connectorv1.ConnectResponse{Sequence: *sequence,
		Frame: &connectorv1.ConnectResponse_TelemetryTunnelDesiredSet{TelemetryTunnelDesiredSet: desired}}); err != nil {
		return err
	}
	*sequence++
	return nil
}

func (gateway Gateway) markUncertain(identity TrustedIdentity, epoch int64, values map[string]activeCommand) {
	for commandID, command := range values {
		status := "delivery_unknown"
		if command.acknowledged {
			status = "result_unknown"
		}
		_, _ = gateway.Service.TransitionCommand(context.Background(), identity, epoch, commandID, status, nil, "CONNECTOR_DISCONNECTED")
	}
}

func removeExpiredActiveCommands(now time.Time, active map[string]activeCommand, acknowledgements map[uint64]string) {
	for commandID, command := range active {
		if command.expiresAt.After(now) {
			continue
		}
		delete(active, commandID)
		for sequence, pendingCommandID := range acknowledgements {
			if pendingCommandID == commandID {
				delete(acknowledgements, sequence)
			}
		}
	}
}

func (gateway Gateway) close(stream connectorv1.ConnectorControlService_ConnectServer, sequence uint64, reason commonv1.CloseReason, code string) error {
	_ = stream.Send(&connectorv1.ConnectResponse{Sequence: sequence, Frame: &connectorv1.ConnectResponse_Close{Close: &commonv1.StreamClose{
		Reason: reason, Error: &commonv1.ErrorStatus{Code: code, MessageKey: "errors.connector.protocol", Retryable: false}}}})
	return fmt.Errorf("connector stream closed: %s", code)
}

func (gateway Gateway) heartbeatInterval() time.Duration {
	if gateway.HeartbeatInterval <= 0 {
		return 30 * time.Second
	}
	return gateway.HeartbeatInterval
}

func (gateway Gateway) dispatchInterval() time.Duration {
	if gateway.DispatchInterval <= 0 {
		return defaultDispatchEvery
	}
	return gateway.DispatchInterval
}

func normalizeResultStatus(value string) string {
	switch strings.ToLower(value) {
	case "succeeded", "success":
		return "succeeded"
	case "failed", "failure":
		return "failed"
	case "running":
		return "running"
	default:
		return "result_unknown"
	}
}

func LoadServerTLS(certPath, keyPath, caPath string) (*tls.Config, error) {
	material, err := tlsmaterial.Load(tlsmaterial.Options{CertificatePath: certPath, PrivateKeyPath: keyPath,
		CABundlePath: caPath, Usage: x509.ExtKeyUsageServerAuth})
	if err != nil {
		return nil, err
	}
	return material.ServerConfig(tls.RequireAndVerifyClientCert, nil)
}
