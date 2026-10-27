package telemetry

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/observationscope"
)

// Resource queries follow the resource page's object-read capability and the
// current data grants. Signal selection never grants or removes access.
func ResourceQueryPermissions() []string {
	return []string{"host.read", "kubernetes.read", "application.read", "monitoring.read"}
}

func (service Service) ResourceQueryActor(ctx context.Context, enterprise, subject uuid.UUID, kind string, version int64) (Actor, error) {
	result := Actor{EnterpriseID: enterprise, SubjectID: subject, SubjectType: kind, AuthorizationVersion: version, AuthorizedResourceIDs: []uuid.UUID{}}
	if service.Store == nil {
		return result, ErrUnavailable
	}
	actor := dashboardaccess.Actor{EnterpriseID: enterprise, SubjectID: subject, SubjectType: kind, AuthorizationVersion: version}
	snapshot, err := (observationscope.Resolver{Queries: service.Store.Queries}).Directory(ctx, actor, observationscope.Selection{Mode: observationscope.AllAuthorized, Signal: "catalog"})
	if errors.Is(err, observationscope.ErrDenied) {
		return result, ErrDenied
	}
	if err != nil {
		return result, err
	}
	result.AuthorizedResourceIDs = snapshot.IDs()
	return result, nil
}

// ResolveQueryScope keeps the authoritative resource type until source lookup;
// a UUID granted for an application cannot select a host's source with that UUID.
func (service Service) ResolveQueryScope(ctx context.Context, actor Actor, ids []uuid.UUID, signal string, types ...string) (observationscope.Snapshot, []string, error) {
	if service.Store == nil {
		return observationscope.Snapshot{}, nil, ErrUnavailable
	}
	scopeSignal := signal
	if signal == "overview" {
		scopeSignal = "catalog"
	}
	a := dashboardaccess.Actor{EnterpriseID: actor.EnterpriseID, SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion}
	snapshot, err := (observationscope.Resolver{Queries: service.Store.Queries}).Resolve(ctx, a, observationscope.Selection{Mode: observationscope.Explicit, IDs: ids, Types: types, Signal: scopeSignal})
	if errors.Is(err, observationscope.ErrDenied) {
		return snapshot, nil, ErrDenied
	}
	if errors.Is(err, observationscope.ErrInvalid) {
		return snapshot, nil, ErrQueryInvalid
	}
	if err != nil {
		return snapshot, nil, err
	}
	if len(snapshot.Resources) == 0 {
		return snapshot, []string{}, nil
	}
	query, err := (observationscope.Resolver{Queries: service.Store.Queries}).Sources(ctx, a, observationscope.SourceSelection{Resources: snapshot.Refs(), Signal: scopeSignal, Applications: snapshot.Selection.Applications})
	if err != nil {
		return snapshot, nil, err
	}
	snapshot.Query = &query
	return snapshot, query.Keys, nil
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

func (service Service) ValidateResolvedQueryScope(ctx context.Context, actor Actor, frozen observationscope.Snapshot) error {
	a := dashboardaccess.Actor{EnterpriseID: actor.EnterpriseID, SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion}
	current, err := (observationscope.Resolver{Queries: service.Store.Queries}).Resolve(ctx, a, frozen.Selection)
	if err != nil {
		if errors.Is(err, observationscope.ErrDenied) {
			return ErrDenied
		}
		if errors.Is(err, observationscope.ErrInvalid) {
			return ErrQueryScope
		}
		return err
	}
	if current.Signature != frozen.Signature {
		return ErrQueryScope
	}
	if frozen.Query != nil {
		in := frozen.Query.Input
		in.Resources = current.Refs()
		in.Keys = frozen.Query.Keys
		resolved, e := (observationscope.Resolver{Queries: service.Store.Queries}).Sources(ctx, a, in)
		if e != nil {
			if errors.Is(e, observationscope.ErrDenied) {
				return ErrDenied
			}
			if errors.Is(e, observationscope.ErrInvalid) {
				return ErrQueryScope
			}
			return e
		}
		if resolved.Signature != frozen.Query.Signature {
			return ErrQueryScope
		}
	}
	return nil
}
