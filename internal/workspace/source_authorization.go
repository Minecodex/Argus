package workspace

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// A shared directory cannot track arbitrary shell copies. Its durable source
// set therefore covers every file, including unregistered derived files.
func (service Service) authorizeWorkspace(ctx context.Context, p toolruntime.Principal, workspace db.Workspace) error {
	if err := service.Authorize(ctx, p); err != nil {
		return err
	}
	if workspace.EnterpriseID != p.EnterpriseID || workspace.ConversationID != p.ConversationID || workspace.Status != "ready" {
		return toolruntime.Error{Kind: "WORKSPACE_UNAVAILABLE"}
	}
	return service.validateSources(ctx, p, workspace.SourceResultRefs)
}

// Register before any bytes can reach the PVC. Keep the dependency even if a
// later write/metadata step fails, since a crash cannot prove absence of bytes.
func (a *Access) registerSource(ctx context.Context, ref string) error {
	if a.principal == nil {
		return toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	}
	if err := a.service.validateSources(ctx, *a.principal, []string{ref}); err != nil {
		return err
	}
	_, err := a.service.Store.Queries.RegisterWorkspaceSource(ctx, db.RegisterWorkspaceSourceParams{
		ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID,
		LeaseOwner: a.Workspace.LeaseOwner, FenceToken: a.Workspace.FenceToken, ResultRef: ref,
	})
	if err != nil {
		return err
	}
	return a.checkAuthorization(ctx)
}

func (a *Access) checkAuthorization(ctx context.Context) error {
	if a.principal == nil { // System cleanup must remain possible after revocation.
		return nil
	}
	current, err := a.service.Store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: a.Workspace.ID, EnterpriseID: a.Workspace.EnterpriseID})
	if err == nil {
		err = a.service.authorizeWorkspace(ctx, *a.principal, current)
	}
	if err != nil {
		a.cancel()
	}
	return err
}

func (service Service) publicationSources(ctx context.Context, p toolruntime.Principal, workspaceID uuid.UUID) ([]string, error) {
	workspace, err := service.Store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: workspaceID, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return nil, err
	}
	return workspace.SourceResultRefs, service.authorizeWorkspace(ctx, p, workspace)
}
