package directexecutor

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/sshtarget"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (executor *Executor) runHostRemovalLoop(ctx context.Context) {
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	service := hostremoval.Service{Store: executor.Store}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reserved := executor.reserveAvailable()
			if reserved == 0 {
				continue
			}
			operations, err := executor.Store.Queries.ClaimDirectHostRemovalOperations(ctx, db.ClaimDirectHostRemovalOperationsParams{Limit: int32(reserved), LeaseOwner: executor.InstanceID})
			if err != nil {
				executor.release(reserved)
				continue
			}
			executor.release(reserved - len(operations))
			for _, operation := range operations {
				go func(item db.HostRemovalOperation) {
					defer executor.release(1)
					executor.executeHostRemoval(ctx, service, item)
				}(operation)
			}
		}
	}
}

func (executor *Executor) executeHostRemoval(parent context.Context, service hostremoval.Service, operation db.HostRemovalOperation) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	leaseDone := make(chan struct{})
	defer close(leaseDone)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-leaseDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				rows, err := executor.Store.Queries.RenewHostRemovalOperationLease(ctx, db.RenewHostRemovalOperationLeaseParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, LeaseOwner: executor.InstanceID})
				if err != nil || rows != 1 {
					cancel()
					return
				}
			}
		}
	}()
	plan, err := hostremoval.DecodeOperationPlan(operation)
	if err != nil {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_PLAN_INVALID", false)
		return
	}
	addresses, err := executor.Validator.Resolve(ctx, plan.Address)
	if err != nil || len(addresses) == 0 {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_TARGET_UNREACHABLE", false)
		return
	}
	credential, err := executor.Store.Queries.GetCredential(ctx, db.GetCredentialParams{ID: plan.CredentialID.UUID, EnterpriseID: operation.EnterpriseID})
	if err != nil || credential.Status != "active" || credential.Version != plan.CredentialVersion {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_CREDENTIAL_UNAVAILABLE", false)
		return
	}
	lease, err := executor.Secrets.IssueLease(secret.WithActorType(ctx, "direct_executor"), executor.InstanceID, operation.EnterpriseID, secret.LeaseRequest{
		CredentialID: credential.ID, OperationRef: "host_removal:" + operation.ID.String(), TargetResourceType: "host", TargetResourceID: operation.HostID,
		RecipientType: "direct_executor", RecipientID: executor.InstanceID, Protocol: "ssh", TTL: secret.MaxLeaseTTL})
	if err != nil {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_CREDENTIAL_UNAVAILABLE", false)
		return
	}
	defer clear(lease.Value)
	client, err := executor.dialPinnedSSH(ctx, addresses[0], plan.Port, plan.Username, plan.PinnedHostKey, lease.Value)
	if err != nil {
		code := "HOST_REMOVAL_TARGET_UNREACHABLE"
		if strings.Contains(strings.ToLower(err.Error()), "host key") || errors.Is(err, errHostKeyMismatch) {
			code = "HOST_REMOVAL_HOST_KEY_CHANGED"
		}
		_ = service.Fail(ctx, operation, code, false)
		return
	}
	defer client.Close()
	stopOnCancel := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopOnCancel()
	if err = executor.Validator.Revalidate(ctx, plan.Address, addresses); err != nil {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_TARGET_CHANGED", false)
		return
	}
	evidence, err := sshtarget.Probe(client, strings.SplitN(plan.TargetPlatform, "_", 2)[0])
	if err != nil || evidence.Platform+"_"+evidence.Architecture != plan.TargetPlatform || !evidence.Privileged {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_PLATFORM_CHANGED", false)
		return
	}
	if err = service.BeginRemote(ctx, operation); err != nil {
		_ = service.Fail(ctx, operation, "TARGET_IDENTITY_CHANGED", false)
		return
	}
	_, canonicalEvidence, cleanupHash, cleanupErr := hostremoval.ExecuteSSH(ctx, client, plan)
	if cleanupErr != nil {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_LOCAL_CLEANUP_UNKNOWN", true)
		return
	}
	_ = executor.Secrets.ConsumeLease(ctx, operation.EnterpriseID, lease.Lease.ID)
	if err = service.FinalizeTrusted(ctx, operation, canonicalEvidence, cleanupHash); err != nil {
		_ = service.Fail(ctx, operation, "HOST_REMOVAL_FINALIZE_FAILED", true)
	}
}
