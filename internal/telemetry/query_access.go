package telemetry

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
)

// Resource queries follow the resource page's object-read capability and the
// current data grants. Signal selection never grants or removes access.
func (service Service) ResourceQueryActor(ctx context.Context, enterprise, subject uuid.UUID, kind string, version int64) (Actor, error) {
	result := Actor{EnterpriseID: enterprise, SubjectID: subject, SubjectType: kind, AuthorizationVersion: version, AuthorizedResourceIDs: []uuid.UUID{}}
	if service.Store == nil {
		return result, ErrUnavailable
	}
	actor := dashboardaccess.Actor{EnterpriseID: enterprise, SubjectID: subject, SubjectType: kind, AuthorizationVersion: version}
	readable := false
	for _, object := range []struct{ kind, permission string }{{"host", "host.read"}, {"kubernetes_cluster", "kubernetes.read"}} {
		if err := dashboardaccess.Authorize(ctx, service.Store.Queries, actor, object.permission); err != nil {
			if errors.Is(err, dashboardaccess.ErrDenied) {
				continue
			}
			return result, err
		}
		readable = true
		ids, err := dashboardaccess.AuthorizedIDs(ctx, service.Store.Queries, actor, object.kind)
		if err != nil {
			return result, err
		}
		result.AuthorizedResourceIDs = append(result.AuthorizedResourceIDs, ids...)
	}
	if !readable {
		return result, ErrDenied
	}
	return result, nil
}

func (service Service) ValidateQueryScope(ctx context.Context, actor Actor, ids []uuid.UUID) error {
	current, err := service.ResourceQueryActor(ctx, actor.EnterpriseID, actor.SubjectID, actor.SubjectType, actor.AuthorizationVersion)
	if err != nil {
		return err
	}
	_, partial, err := service.AuthorizedResources(ctx, current, ids)
	if err == nil && partial {
		return ErrQueryScope
	}
	return err
}
