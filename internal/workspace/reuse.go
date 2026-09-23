package workspace

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/sandbox"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"github.com/kakj-go/Argus/internal/workspacesupervisor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (service Service) leaseSeconds(runtime sandbox.WorkspaceRuntime, idle bool) int {
	seconds := max(1, int(service.Config.IdleTTL.Seconds()))
	if !idle {
		seconds += 360
	}
	return max(1, min(seconds, int(runtime.Profile.TimeoutSeconds)))
}

func (service Service) tryReuse(a *Access, compute bool, scope accessScope) bool {
	w := a.Workspace
	if !w.ActivePodName.Valid {
		return false
	}
	pod, err := service.Kubernetes.Client.CoreV1().Pods(w.Namespace).Get(a.Context, w.ActivePodName.String, metav1.GetOptions{})
	if err != nil || !readyWorkspacePod(pod) || pod.Labels[workspaceLabel] != w.ID.String() {
		return false
	}
	generation, err := strconv.ParseInt(pod.Labels[fenceLabel], 10, 64)
	if err != nil || generation <= 0 || generation > w.FenceToken {
		return false
	}
	ioOnly := pod.Labels["argus.io/workspace-mode"] == "io"
	if compute && ioOnly {
		return false
	}
	if !ioOnly {
		profile, err := uuid.Parse(pod.Labels["argus.io/runtime-profile-id"])
		if err != nil {
			return false
		}
		runtime, err := service.Sandbox.WorkspaceRuntimeByID(a.Context, profile)
		if err != nil {
			return false
		}
		if runtime.Identity().Label() != pod.Labels["argus.io/runtime-version"] || scope.Runtime != nil && runtime.Identity() != *scope.Runtime {
			return false
		}
		sessions, err := service.Store.Queries.ListWorkspaceSandboxSessions(a.Context, db.ListWorkspaceSandboxSessionsParams{WorkspaceID: uuid.NullUUID{UUID: w.ID, Valid: true}, EnterpriseID: w.EnterpriseID})
		if err != nil {
			return false
		}
		for _, session := range sessions {
			if session.UpstreamSessionID == w.ActiveSandboxID.String && session.Status == "running" && session.ExpiresAt.Time.After(time.Now()) {
				a.SessionID = session.ID
				a.SandboxID = session.UpstreamSessionID
				a.Runtime = &runtime
				break
			}
		}
		if a.Runtime == nil {
			return false
		}
		if err := service.Sandbox.RenewWorkspaceSession(a.Context, a.SessionID, runtime, service.leaseSeconds(runtime, false)); err != nil {
			return false
		}
	}
	if err := service.connectAccess(a, pod, generation); err != nil {
		a.disconnect()
		return false
	}
	a.reusable = true
	return true
}

func readyWorkspacePod(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (service Service) connectAccess(a *Access, pod *corev1.Pod, generation int64) error {
	config, err := service.clientTLS()
	if err != nil {
		return err
	}
	if a.Runtime != nil {
		a.Supervisor, err = workspacesupervisor.Dial(net.JoinHostPort(pod.Status.PodIP, "8448"), config, a.Workspace.ID.String(), a.Workspace.FenceToken)
		if err != nil {
			return err
		}
		if err := a.Supervisor.Bind(a.Context, generation); err != nil {
			return err
		}
	}
	a.IO, err = workspaceio.Dial(net.JoinHostPort(pod.Status.PodIP, "8447"), config, a.Workspace.ID.String(), a.Workspace.FenceToken)
	if err != nil {
		return err
	}
	return a.IO.Bind(a.Context, generation)
}

func (a *Access) disconnect() {
	if a.IO != nil {
		_ = a.IO.Close()
		a.IO = nil
	}
	if a.Supervisor != nil {
		_ = a.Supervisor.Close()
		a.Supervisor = nil
	}
}

func (a *Access) park(ctx context.Context) bool {
	if err := a.checkAuthorization(ctx); err != nil {
		return false
	}
	if !a.reusable || a.Context.Err() != nil {
		return false
	}
	current, err := a.service.Store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID})
	if err != nil || current.Status != "ready" || current.LeaseOwner.String != a.owner || current.FenceToken != a.Workspace.FenceToken {
		return false
	}
	if a.Supervisor != nil {
		if err := a.Supervisor.Release(ctx); err != nil {
			return false
		}
	}
	if a.IO == nil || a.IO.Release(ctx) != nil {
		return false
	}
	if a.Runtime != nil {
		if err := a.service.Sandbox.RenewWorkspaceSession(ctx, a.SessionID, *a.Runtime, a.service.leaseSeconds(*a.Runtime, true)); err != nil {
			return false
		}
	}
	// Warm operations must reset the durable idle clock as well as the local
	// RPC watchdogs. Otherwise the reconciler retires a frequently reused Pod
	// based on the timestamp of its original attachment.
	count, err := a.service.Store.Queries.SetWorkspaceAttachment(ctx, db.SetWorkspaceAttachmentParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID, LeaseOwner: current.LeaseOwner, FenceToken: a.Workspace.FenceToken, ActivePodName: current.ActivePodName, ActiveSandboxID: current.ActiveSandboxID})
	return err == nil && count == 1
}

func (a *Access) finish(ctx context.Context) error {
	if a.park(ctx) {
		a.disconnect()
		a.cancel()
		<-a.renewed
		_, err := a.service.Store.Queries.ReleaseWorkspaceLease(ctx, db.ReleaseWorkspaceLeaseParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: a.owner, Valid: true}, FenceToken: a.Workspace.FenceToken})
		return err
	}
	a.disconnect()
	a.cancel()
	<-a.renewed
	// Retiring a shared warm workload requires a currently owned lease. Reserve
	// longer than Close's 60-second deadline before dispatching Kubernetes
	// mutations; an expired/stale caller must leave cleanup to the new owner.
	owned, renewErr := a.service.Store.Queries.RenewWorkspaceLease(ctx, db.RenewWorkspaceLeaseParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: a.owner, Valid: true}, FenceToken: a.Workspace.FenceToken, Column5: pgtype.Interval{Microseconds: 90_000_000, Valid: true}})
	if renewErr != nil || owned != 1 {
		if renewErr != nil {
			return renewErr
		}
		return toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
	}
	err := a.service.Kubernetes.Retire(ctx, a.Workspace)
	if err != nil {
		return err
	}
	// The old workload is now physically gone. Metadata failures may release
	// the lease for an immediate retry; a future owner cannot reuse that Pod.
	err = a.service.finishSessions(ctx, a.Workspace)
	if err == nil {
		var changed int64
		changed, err = a.service.Store.Queries.SetWorkspaceAttachment(ctx, db.SetWorkspaceAttachmentParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: a.owner, Valid: true}, FenceToken: a.Workspace.FenceToken})
		if err == nil && changed != 1 {
			err = toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
		}
	}
	_, releaseErr := a.service.Store.Queries.ReleaseWorkspaceLease(ctx, db.ReleaseWorkspaceLeaseParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: a.owner, Valid: true}, FenceToken: a.Workspace.FenceToken})
	return errors.Join(err, releaseErr)
}
