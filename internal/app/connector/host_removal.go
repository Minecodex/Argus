package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/types/known/anypb"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/sshtarget"
)

func executeHostConnectorRemoval(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.HostConnectorRemovalResult, error) {
	var request connectorv1.HostConnectorRemoval
	if payload == nil || payload.UnmarshalTo(&request) != nil || request.GetOperationId() == "" || request.GetHostId() == "" ||
		request.GetConnectorId() == "" || request.GetRemovalGeneration() == 0 || request.GetAddress() == "" || request.GetPort() == 0 ||
		request.GetUsername() == "" || request.GetPinnedHostKey() == "" || len(credential) == 0 {
		return nil, errors.New("Host Connector removal command is invalid")
	}
	operationID, operationErr := uuid.Parse(request.GetOperationId())
	hostID, hostErr := uuid.Parse(request.GetHostId())
	connectorID, connectorErr := uuid.Parse(request.GetConnectorId())
	if operationErr != nil || hostErr != nil || connectorErr != nil || request.GetTargetPlatform() != "linux_amd64" && request.GetTargetPlatform() != "linux_arm64" && request.GetTargetPlatform() != "windows_amd64" {
		return nil, errors.New("Host Connector removal identity is invalid")
	}
	auth, err := connectorSSHAuth(credential)
	if err != nil {
		return nil, err
	}
	configuration := &ssh.ClientConfig{User: request.GetUsername(), Auth: []ssh.AuthMethod{auth}, Timeout: commandTimeout,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != request.GetPinnedHostKey() {
				return errors.New("Host key changed")
			}
			return nil
		}}
	client, err := ssh.Dial("tcp", net.JoinHostPort(request.GetAddress(), fmt.Sprint(request.GetPort())), configuration)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	stopOnCancel := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopOnCancel()
	platform := strings.SplitN(request.GetTargetPlatform(), "_", 2)[0]
	evidence, err := sshtarget.Probe(client, platform)
	if err != nil || evidence.Platform+"_"+evidence.Architecture != request.GetTargetPlatform() || !evidence.Privileged {
		return nil, errors.New("Host Connector removal target changed")
	}
	plan := hostremoval.Plan{OperationID: uuid.NullUUID{UUID: operationID, Valid: true}, HostID: hostID, ConnectorID: connectorID,
		RemovalGeneration: int64(request.GetRemovalGeneration()), TargetPlatform: request.GetTargetPlatform()}
	if request.GetManagedChangeId() != "" {
		changeID, parseErr := uuid.Parse(request.GetManagedChangeId())
		if parseErr != nil || !json.Valid(request.GetManagedChangeBeforeJson()) || !json.Valid(request.GetManagedChangeAppliedJson()) {
			return nil, errors.New("Host removal RDP snapshot is invalid")
		}
		plan.ManagedChangeID = uuid.NullUUID{UUID: changeID, Valid: true}
		plan.ManagedChangeBefore = request.GetManagedChangeBeforeJson()
		plan.ManagedChangeApplied = request.GetManagedChangeAppliedJson()
	}
	cleanupEvidence, canonicalEvidence, cleanupHash, err := hostremoval.ExecuteSSH(ctx, client, plan)
	if err != nil {
		return nil, err
	}
	return &connectorv1.HostConnectorRemovalResult{OperationId: operationID.String(), CleanupHash: cleanupHash, CleanupEvidenceJson: canonicalEvidence,
		LocalCleanupVerified: true, LocalConfigDrift: cleanupEvidence.RDPConfigStatus == "drifted"}, nil
}
