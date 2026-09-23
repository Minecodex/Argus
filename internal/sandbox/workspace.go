package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/opensandbox"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type WorkspaceRuntime struct {
	Backend db.SandboxBackend
	Profile db.SandboxProfile
	Image   db.SandboxImage
	Client  *opensandbox.Client
}

func (service Service) SelectWorkspaceRuntime(ctx context.Context) (WorkspaceRuntime, error) {
	profile, err := service.Store.Queries.SelectSandboxProfile(ctx, "agent_workspace")
	if err != nil {
		return WorkspaceRuntime{}, err
	}
	return service.WorkspaceRuntimeByID(ctx, profile.ID)
}

func (service Service) ReserveWorkspaceSession(ctx context.Context, enterpriseID, workspaceID, toolCallID uuid.UUID, profile db.SandboxProfile, seconds int) (db.SandboxSession, error) {
	var result db.SandboxSession
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		quota, err := q.GetSandboxQuotaForUpdate(ctx, enterpriseID)
		if err != nil {
			return ErrQuotaExceeded
		}
		active, err := q.CountActiveSandboxSessions(ctx, enterpriseID)
		if err != nil {
			return err
		}
		committed, err := q.GetSandboxMonthlyCommittedSeconds(ctx, db.GetSandboxMonthlyCommittedSecondsParams{EnterpriseID: enterpriseID, Month: pgtype.Date{Time: monthStart(time.Now().UTC()), Valid: true}})
		if err != nil {
			return err
		}
		remaining, allowed := sandboxReservationSeconds(quota.MonthlySessionSeconds, committed, int64(seconds))
		if active >= quota.MaxConcurrentSessions || !allowed || remaining < int64(seconds) {
			return ErrQuotaExceeded
		}
		result, err = q.CreateWorkspaceSandboxSession(ctx, db.CreateWorkspaceSandboxSessionParams{ID: toolCallID, EnterpriseID: enterpriseID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
			ToolCallID: uuid.NullUUID{UUID: toolCallID, Valid: true}, ProfileID: profile.ID, ProfileRevision: profile.Revision, UpstreamSessionID: "pending:" + toolCallID.String(), ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Duration(seconds) * time.Second), Valid: true}})
		return err
	})
	return result, err
}

func (service Service) FinishWorkspaceSession(ctx context.Context, id uuid.UUID) (db.SandboxSession, error) {
	return service.finalizeSession(ctx, id, "terminated")
}

func (runtime WorkspaceRuntime) Create(ctx context.Context, workspace db.Workspace, toolCallID uuid.UUID, seconds int) (opensandbox.Sandbox, error) {
	if runtime.Client == nil {
		return opensandbox.Sandbox{}, errors.New("sandbox runtime is unavailable")
	}
	return runtime.Client.Create(ctx, opensandbox.CreateRequest{Image: opensandbox.ImageSpec{URI: runtime.Image.ImageRef + "@" + runtime.Image.Digest}, Timeout: seconds,
		ResourceLimits: map[string]string{"cpu": fmt.Sprintf("%dm", runtime.Profile.CpuMillis), "memory": fmt.Sprintf("%dMi", runtime.Profile.MemoryMib)},
		Metadata:       map[string]string{"argus.io/workspace-id": workspace.ID.String(), "argus.io/enterprise-id": workspace.EnterpriseID.String(), "argus.io/workspace-fence": fmt.Sprint(workspace.FenceToken), "argus.io/tool-call-id": toolCallID.String(), "argus.io/runtime-profile-id": runtime.Profile.ID.String(), "argus.io/runtime-version": runtime.Identity().Label()},
		NetworkPolicy:  map[string]any{"defaultAction": "deny", "egress": []any{}}, Volumes: []opensandbox.Volume{{Name: "workspace", PVC: opensandbox.PVC{ClaimName: workspace.PvcName}, MountPath: "/workspace"}},
		Entrypoint: []string{"sleep", "infinity"}, Env: map[string]string{"HOME": "/home/argus"}})
}
