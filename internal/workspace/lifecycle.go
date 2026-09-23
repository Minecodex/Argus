package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Delete records explicit intent. Cleanup waits for the current writer and is
// retried durably; archive, run completion and idle expiry never call this path.
func (service Service) Delete(ctx context.Context, p toolruntime.Principal, key string) (db.Workspace, error) {
	if err := service.Authorize(ctx, p); err != nil {
		return db.Workspace{}, err
	}
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", p.UserID.String(), "workspace.delete", key, p.ConversationID, 202, func(q *db.Queries) (db.Workspace, error) {
		if _, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID}); err != nil {
			return db.Workspace{}, err
		}
		current, err := q.GetConversationWorkspace(ctx, db.GetConversationWorkspaceParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return current, err
		}
		if current.Status == "deleting" {
			return current, nil
		}
		runs, err := q.CancelConversationAgentRuns(ctx, db.CancelConversationAgentRunsParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return current, err
		}
		for _, run := range runs {
			if _, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "run_state_changed", ActorType: "user", ActorID: p.UserID.String(), Payload: map[string]any{"status": "cancelled", "stop_reason": "workspace_deleted"}, Classification: "internal"}); err != nil {
				return current, err
			}
		}
		return q.SetWorkspaceStatus(ctx, db.SetWorkspaceStatusParams{ID: current.ID, EnterpriseID: p.EnterpriseID, Status: "deleting", Version: current.Version})
	})
}

func (service Service) RunReconciler(ctx context.Context, logger *slog.Logger) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if err := service.Reconcile(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("Workspace reconciliation failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (service Service) reconcileWorkspaces(ctx context.Context) error {
	deleting, err := service.Store.Queries.ListDeletingWorkspaces(ctx, 20)
	if err != nil {
		return err
	}
	for _, workspace := range deleting {
		if err := service.cleanup(ctx, workspace); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	idle, err := service.Store.Queries.ListIdleWorkspaces(ctx, db.ListIdleWorkspacesParams{LastUsedAt: pgtype.Timestamptz{Time: time.Now().Add(-service.Config.IdleTTL), Valid: true}, Limit: 20})
	if err != nil {
		return err
	}
	for _, workspace := range idle {
		owner := uuid.NewString()
		current, err := service.Store.Queries.ClaimWorkspaceLease(ctx, db.ClaimWorkspaceLeaseParams{ExpectedFence: workspace.FenceToken, ID: workspace.ID, EnterpriseID: workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: owner, Valid: true}, Column4: pgtype.Interval{Microseconds: 30_000_000, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		a := service.cleanupAccess(ctx, current, owner)
		if err := a.Close(); err != nil {
			return err
		}
		if err := service.finishSessions(ctx, current); err != nil {
			return err
		}
	}
	_, err = service.Store.Queries.ExpireWorkspaceUploads(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (service Service) Reconcile(ctx context.Context) error {
	if service.Config.Enabled {
		if err := service.reconcileWorkspaces(ctx); err != nil {
			return err
		}
	}
	deleted, err := service.Store.Queries.ListDeletedConversationsForCleanup(ctx, 20)
	if err != nil {
		return err
	}
	for _, item := range deleted {
		if service.Objects == nil {
			return toolruntime.Error{Kind: "WORKSPACE_STORAGE_UNAVAILABLE"}
		}
		if err := service.Objects.DeletePrefix(ctx, fmt.Sprintf("enterprises/%s/conversations/%s/", item.EnterpriseID, item.ID)); err != nil {
			return err
		}
		if err := service.Store.Queries.FinishConversationFileCleanup(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (service Service) cleanupAccess(ctx context.Context, workspace db.Workspace, owner string) *Access {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	a := &Access{Context: ctx, Workspace: workspace, service: service, owner: owner, cancel: cancel, renewed: make(chan struct{}), allowDeleting: workspace.Status == "deleting"}
	go a.renew()
	return a
}

func (service Service) finishSessions(ctx context.Context, workspace db.Workspace) error {
	sessions, err := service.Store.Queries.ListWorkspaceSandboxSessions(ctx, db.ListWorkspaceSandboxSessionsParams{WorkspaceID: uuid.NullUUID{UUID: workspace.ID, Valid: true}, EnterpriseID: workspace.EnterpriseID})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if _, err := service.Sandbox.FinishWorkspaceSession(ctx, session.ID); err != nil {
			return err
		}
	}
	return nil
}

func (service Service) cleanup(ctx context.Context, workspace db.Workspace) error {
	owner := uuid.NewString()
	current, err := service.Store.Queries.ClaimWorkspaceDeletion(ctx, db.ClaimWorkspaceDeletionParams{ID: workspace.ID, EnterpriseID: workspace.EnterpriseID, LeaseOwner: pgtype.Text{String: owner, Valid: true}, Column4: pgtype.Interval{Microseconds: 30_000_000, Valid: true}})
	if err != nil {
		return err
	}
	access := service.cleanupAccess(ctx, current, owner)
	defer access.Close()
	ctx = access.Context
	if err := service.Kubernetes.Retire(ctx, current); err != nil {
		return err
	}
	if err := service.finishSessions(ctx, current); err != nil {
		return err
	}
	if err := service.Kubernetes.DeleteVolume(ctx, current); err != nil {
		return err
	}
	if service.Objects == nil {
		return toolruntime.Error{Kind: "WORKSPACE_STORAGE_UNAVAILABLE"}
	}
	if err := service.Objects.DeletePrefix(ctx, objectPrefix(current)); err != nil {
		return err
	}
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		count, err := q.FinishWorkspaceDeletion(ctx, db.FinishWorkspaceDeletionParams{ID: current.ID, EnterpriseID: current.EnterpriseID, LeaseOwner: pgtype.Text{String: owner, Valid: true}, FenceToken: current.FenceToken})
		if err != nil {
			return err
		}
		if count != 1 {
			return toolruntime.Error{Kind: "WORKSPACE_LEASE_LOST"}
		}
		if err := q.DeleteWorkspaceFileRecords(ctx, db.DeleteWorkspaceFileRecordsParams{WorkspaceID: current.ID, EnterpriseID: current.EnterpriseID}); err != nil {
			return err
		}
		if err := q.DeleteWorkspaceDeliveryRecords(ctx, db.DeleteWorkspaceDeliveryRecordsParams{WorkspaceID: current.ID, EnterpriseID: current.EnterpriseID}); err != nil {
			return err
		}
		if err := q.FailDeletedWorkspaceUploads(ctx, db.FailDeletedWorkspaceUploadsParams{WorkspaceID: current.ID, EnterpriseID: current.EnterpriseID}); err != nil {
			return err
		}
		return q.ReleaseWorkspaceCapacity(ctx, db.ReleaseWorkspaceCapacityParams{EnterpriseID: current.EnterpriseID, ReservedBytes: current.CapacityBytes})
	})
}

func (runtime Kubernetes) DeleteVolume(ctx context.Context, workspace db.Workspace) error {
	claims := runtime.Client.CoreV1().PersistentVolumeClaims(workspace.Namespace)
	claim, err := claims.Get(ctx, workspace.PvcName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		if claim.Labels[workspaceLabel] != workspace.ID.String() || claim.Labels["argus.io/enterprise-id"] != workspace.EnterpriseID.String() {
			return toolruntime.Error{Kind: "WORKSPACE_STORAGE_FORBIDDEN"}
		}
		if err := claims.Delete(ctx, claim.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &claim.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := claims.Get(ctx, workspace.PvcName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if apierrors.IsNotFound(err) {
			remaining := false
			cursor := ""
			for {
				volumes, err := runtime.Client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{Limit: 200, Continue: cursor})
				if err != nil {
					return err
				}
				for _, volume := range volumes.Items {
					ref := volume.Spec.ClaimRef
					if ref != nil && ref.Namespace == workspace.Namespace && ref.Name == workspace.PvcName {
						remaining = true
					}
				}
				if remaining || volumes.Continue == "" {
					break
				}
				cursor = volumes.Continue
			}
			if !remaining {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
