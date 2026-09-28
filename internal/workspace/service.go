// Package workspace owns persistent conversation files independently of compute.
package workspace

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/sandbox"
	"github.com/kakj-go/Argus/internal/storage/objectstore"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"github.com/kakj-go/Argus/internal/workspacesupervisor"
)

type Service struct {
	Store          *postgres.Store
	Idempotency    postgres.Idempotency
	Sandbox        sandbox.Service
	Kubernetes     Kubernetes
	Config         config.Workspace
	Objects        *objectstore.Client
	ExternalSource func(context.Context, toolruntime.Principal, string) (bool, error)
}

// A tool operation is tied to its Run; user file operations are tied to the
// Workspace selected when their upload/file record was read.
type accessScope struct {
	RunID, WorkspaceID uuid.UUID
	Runtime            *sandbox.WorkspaceIdentity
	// Only trusted query-file imports set this. Model tool execution keeps the
	// active-Run requirement; durable transfer may finish after its Run returns.
	BackgroundTransfer bool
}

func workspaceRunAllowed(run db.Run, p toolruntime.Principal, scope accessScope) bool {
	if run.EnterpriseID != p.EnterpriseID || run.ConversationID != p.ConversationID || run.ActorUserID != p.UserID || run.Status == "cancelled" || run.Status == "timed_out" {
		return false
	}
	return scope.BackgroundTransfer || (run.Status != "failed" && run.Status != "succeeded")
}

func (service Service) Authorize(ctx context.Context, p toolruntime.Principal) error {
	if !p.Allows("workspace.use") {
		return toolruntime.Error{Kind: "WORKSPACE_FORBIDDEN"}
	}
	return (conversation.Service{Store: service.Store}).ValidateToolPrincipal(ctx, p)
}

func (service Service) Get(ctx context.Context, p toolruntime.Principal) (db.Workspace, error) {
	if err := service.Authorize(ctx, p); err != nil {
		return db.Workspace{}, err
	}
	return service.Store.Queries.GetConversationWorkspace(ctx, db.GetConversationWorkspaceParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
}

func (service Service) ensure(ctx context.Context, p toolruntime.Principal, scope accessScope) (db.Workspace, error) {
	if !service.Config.Enabled {
		return db.Workspace{}, toolruntime.Error{Kind: "WORKSPACE_NOT_CONFIGURED"}
	}
	if err := service.Authorize(ctx, p); err != nil {
		return db.Workspace{}, err
	}
	if err := service.Kubernetes.Ready(ctx); err != nil {
		return db.Workspace{}, err
	}
	var workspace db.Workspace
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		currentConversation, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err != nil {
			return err
		}
		if currentConversation.Status == "deleted" {
			return toolruntime.Error{Kind: "WORKSPACE_UNAVAILABLE"}
		}
		if scope.RunID != uuid.Nil {
			run, err := q.GetRun(ctx, db.GetRunParams{ID: scope.RunID, EnterpriseID: p.EnterpriseID})
			if err != nil || !workspaceRunAllowed(run, p, scope) {
				return toolruntime.Error{Kind: "TOOL_CANCELLED"}
			}
		}
		workspace, err = q.GetConversationWorkspace(ctx, db.GetConversationWorkspaceParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
		if scope.WorkspaceID != uuid.Nil && (err != nil || workspace.ID != scope.WorkspaceID) {
			return toolruntime.Error{Kind: "WORKSPACE_UNAVAILABLE"}
		}
		if err == nil {
			if workspace.Status == "deleting" {
				return toolruntime.Error{Kind: "WORKSPACE_UNAVAILABLE"}
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := q.EnsureWorkspaceQuota(ctx, db.EnsureWorkspaceQuotaParams{EnterpriseID: p.EnterpriseID, LimitBytes: service.Config.EnterpriseBytes}); err != nil {
			return err
		}
		if _, err := q.ReserveWorkspaceCapacity(ctx, db.ReserveWorkspaceCapacityParams{EnterpriseID: p.EnterpriseID, ReservedBytes: service.Config.DefaultBytes}); err != nil {
			return toolruntime.Error{Kind: "WORKSPACE_QUOTA_EXCEEDED"}
		}
		id := uuid.New()
		workspace, err = q.CreateWorkspace(ctx, db.CreateWorkspaceParams{ID: id, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, PvcName: "workspace-" + id.String(), Namespace: service.Config.Namespace,
			CapacityBytes: service.Config.DefaultBytes, EnvironmentVersion: "argus-workspace/v1"})
		return err
	})
	if err != nil {
		return db.Workspace{}, err
	}
	if workspace.Status == "provisioning" {
		owner := uuid.NewString()
		workspace, err = service.Store.Queries.ClaimWorkspaceProvision(ctx, db.ClaimWorkspaceProvisionParams{ID: workspace.ID, EnterpriseID: workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: owner, Valid: true}, Column4: pgtype.Interval{Microseconds: 30_000_000, Valid: true}})
		if err != nil {
			return workspace, toolruntime.Error{Kind: "WORKSPACE_BUSY"}
		}
		access := service.cleanupAccess(ctx, workspace, owner)
		defer access.Close()
		if err := service.Kubernetes.EnsureVolume(access.Context, workspace); err != nil {
			return workspace, err
		}
		updated, err := service.Store.Queries.SetWorkspaceStatus(ctx, db.SetWorkspaceStatusParams{ID: workspace.ID, EnterpriseID: workspace.EnterpriseID, Status: "ready", Version: workspace.Version})
		if err != nil {
			return workspace, err
		}
		workspace = updated
		if err := access.Close(); err != nil {
			return workspace, err
		}
	}
	return workspace, service.authorizeWorkspace(ctx, p, workspace)
}

type Access struct {
	Context       context.Context
	Workspace     db.Workspace
	IO            *workspaceio.Client
	Supervisor    *workspacesupervisor.Client
	Runtime       *sandbox.WorkspaceRuntime
	SandboxID     string
	SessionID     uuid.UUID
	service       Service
	owner         string
	cancel        context.CancelFunc
	renewed       chan struct{}
	closed        bool
	allowDeleting bool
	reusable      bool
	principal     *toolruntime.Principal
}

func (service Service) access(ctx context.Context, p toolruntime.Principal, compute bool, toolCallID uuid.UUID, scope accessScope) (*Access, error) {
	if compute {
		if scope.Runtime == nil {
			return nil, toolruntime.Error{Kind: "TOOL_VERSION_UNAVAILABLE"}
		}
		if _, err := service.Sandbox.ResolveWorkspaceRuntime(ctx, *scope.Runtime); err != nil {
			return nil, toolruntime.Error{Kind: "TOOL_VERSION_UNAVAILABLE"}
		}
	}
	workspace, err := service.ensure(ctx, p, scope)
	if err != nil {
		return nil, err
	}
	// Only a cleanly released workload is eligible for warm reuse. An expired
	// owner may still have cleanup requests in flight; retire that Pod so its
	// immutable UID can never identify the next owner's workload.
	warm := !workspace.LeaseOwner.Valid
	owner := uuid.NewString()
	workspace, err = service.Store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ExpectedFence: workspace.FenceToken, ID: workspace.ID, EnterpriseID: p.EnterpriseID, LeaseOwner: pgtype.Text{String: owner, Valid: true}, Column4: pgtype.Interval{Microseconds: 30_000_000, Valid: true}})
	if err != nil {
		return nil, toolruntime.Error{Kind: "WORKSPACE_BUSY"}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	a := &Access{Context: ctx, Workspace: workspace, service: service, owner: owner, cancel: cancel, renewed: make(chan struct{}), principal: &p}
	go a.renew()
	fail := func(err error) (*Access, error) { _ = a.Close(); return nil, err }
	if err := a.checkAuthorization(ctx); err != nil {
		return fail(err)
	}
	if warm && service.tryReuse(a, compute, scope) {
		if err := a.checkAuthorization(ctx); err != nil {
			return fail(err)
		}
		return a, nil
	}
	a.disconnect()
	if err := service.Kubernetes.Retire(ctx, workspace); err != nil {
		return fail(err)
	}
	if err := service.finishSessions(ctx, workspace); err != nil {
		return fail(err)
	}
	a.Runtime = nil
	a.SandboxID = ""
	a.SessionID = uuid.Nil
	if compute {
		if scope.Runtime == nil {
			return fail(toolruntime.Error{Kind: "TOOL_VERSION_UNAVAILABLE"})
		}
		selection, err := service.Sandbox.ResolveWorkspaceRuntime(ctx, *scope.Runtime)
		if err != nil {
			return fail(toolruntime.Error{Kind: "TOOL_VERSION_UNAVAILABLE"})
		}
		seconds := max(60, service.leaseSeconds(selection, false))
		session, err := service.Sandbox.ReserveWorkspaceSession(ctx, p.EnterpriseID, workspace.ID, toolCallID, selection.Profile, seconds)
		if err != nil {
			return fail(err)
		}
		a.SessionID = session.ID
		// Reservation and network dispatch are separate steps; revalidate after
		// acquiring quota so a concurrent platform edit cannot change the run.
		selection, err = service.Sandbox.ResolveWorkspaceRuntime(ctx, *scope.Runtime)
		if err != nil {
			return fail(toolruntime.Error{Kind: "TOOL_VERSION_UNAVAILABLE"})
		}
		created, err := selection.Create(ctx, workspace, toolCallID, seconds)
		if err != nil {
			return fail(err)
		}
		a.Runtime = &selection
		a.SandboxID = created.ID
		if _, err := service.Store.Queries.SetWorkspaceSandboxUpstream(ctx, db.SetWorkspaceSandboxUpstreamParams{ID: session.ID, EnterpriseID: p.EnterpriseID, UpstreamSessionID: created.ID}); err != nil {
			return fail(err)
		}
	} else {
		if _, err := service.Kubernetes.CreateIO(ctx, workspace); err != nil {
			return fail(err)
		}
	}
	pod, err := service.Kubernetes.WaitPod(ctx, workspace)
	if err != nil {
		return fail(err)
	}
	count, err := service.Store.Queries.SetWorkspaceAttachment(ctx, db.SetWorkspaceAttachmentParams{ID: workspace.ID, EnterpriseID: p.EnterpriseID, LeaseOwner: pgtype.Text{String: owner, Valid: true}, FenceToken: workspace.FenceToken,
		ActivePodName: pgtype.Text{String: pod.Name, Valid: true}, ActiveSandboxID: pgtype.Text{String: a.SandboxID, Valid: a.SandboxID != ""}})
	if err != nil {
		return fail(err)
	}
	if count != 1 {
		return fail(toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"})
	}
	if err := service.connectAccess(a, pod, workspace.FenceToken); err != nil {
		return fail(err)
	}
	a.reusable = true
	if err := a.checkAuthorization(ctx); err != nil {
		return fail(err)
	}
	return a, nil
}

func (service Service) clientTLS() (*tls.Config, error) {
	return workspaceio.TLS(service.Config.ClientCert, service.Config.ClientKey, service.Config.CAPath, service.Config.ServerName, false, "")
}

func (a *Access) renew() {
	defer close(a.renewed)
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	authorization := time.NewTicker(time.Second)
	defer authorization.Stop()
	for {
		select {
		case <-a.Context.Done():
			return
		case <-authorization.C:
			ctx, cancel := context.WithTimeout(a.Context, 5*time.Second)
			err := a.checkAuthorization(ctx)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(a.Context, 5*time.Second)
			current, getErr := a.service.Store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID})
			if getErr != nil || current.Status == "deleted" || current.Status == "deleting" && !a.allowDeleting {
				cancel()
				a.cancel()
				return
			}
			count, err := a.service.Store.Queries.RenewWorkspaceLease(ctx, db.RenewWorkspaceLeaseParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: a.owner, Valid: true}, FenceToken: a.Workspace.FenceToken, Column5: pgtype.Interval{Microseconds: 30_000_000, Valid: true}})
			cancel()
			if err != nil || count != 1 {
				a.cancel()
				return
			}
		}
	}
}

func (a *Access) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return a.finish(ctx)
}

func objectPrefix(workspace db.Workspace) string {
	return fmt.Sprintf("enterprises/%s/workspaces/%s/", workspace.EnterpriseID, workspace.ID)
}
