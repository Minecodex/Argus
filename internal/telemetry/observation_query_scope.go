package telemetry

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
)

// QueryScope is a selection authority, never a label matcher or a persisted query.
type QueryScope struct {
	ResourceTypes []string          `json:"resource_types,omitempty"`
	Mode          string            `json:"mode"`
	Resources     []resourceref.Ref `json:"resources"`
	SourceType    string            `json:"source_type,omitempty"`
	SourceKeys    []string          `json:"source_keys,omitempty"`
}

func (s QueryScope) Validate() error {
	for _, kind := range s.ResourceTypes {
		if !resourceref.ValidType(kind) {
			return ErrQueryInvalid
		}
	}
	if len(s.ResourceTypes) > 5 {
		return ErrQueryInvalid
	}
	for _, ref := range s.Resources {
		if len(s.ResourceTypes) > 0 && !slices.Contains(s.ResourceTypes, ref.Type) {
			return ErrQueryInvalid
		}
	}
	refs, err := resourceref.Normalize(s.Resources)
	if err != nil || len(refs) != len(s.Resources) || !slices.Contains([]string{observationscope.AllAuthorized, observationscope.Explicit}, s.Mode) || s.Mode == observationscope.AllAuthorized && len(refs) > 0 || len(s.SourceType) > 128 || len(s.SourceKeys) > 10000 {
		return ErrQueryInvalid
	}
	return nil
}

func (s Service) ResolveStructuredQueryScope(ctx context.Context, a Actor, scope QueryScope, signal string) (observationscope.Snapshot, []string, error) {
	var empty observationscope.Snapshot
	if s.Store == nil {
		return empty, nil, ErrUnavailable
	}
	if err := scope.Validate(); err != nil {
		return empty, nil, err
	}
	refs, _ := resourceref.Normalize(scope.Resources)
	types := slices.Clone(scope.ResourceTypes)
	for _, ref := range refs {
		if !slices.Contains(types, ref.Type) {
			types = append(types, ref.Type)
		}
	}
	actor := authorization.Actor{EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion}
	resolver := observationscope.Resolver{Queries: s.Store.Queries}
	snapshot, err := resolver.Resolve(ctx, actor, observationscope.Selection{Mode: scope.Mode, IDs: resourceref.IDs(refs), Types: types, Signal: signal})
	if err != nil {
		return snapshot, nil, queryScopeError(err)
	}
	// Resolve must retain each (type, UUID) pair, not merely match an ID union.
	if scope.Mode == observationscope.Explicit && !slices.Equal(refs, snapshot.Refs()) {
		return snapshot, nil, ErrDenied
	}
	source, err := resolver.Sources(ctx, actor, observationscope.SourceSelection{Resources: snapshot.Refs(), Signal: signal, SourceType: scope.SourceType, Keys: scope.SourceKeys})
	if err != nil {
		return snapshot, nil, queryScopeError(err)
	}
	snapshot.Query = &source
	return snapshot, source.Keys, nil
}

func queryScopeError(err error) error {
	if errors.Is(err, observationscope.ErrDenied) {
		return ErrDenied
	}
	if errors.Is(err, observationscope.ErrInvalid) {
		return ErrQueryInvalid
	}
	return err
}

// UUID-only callers retain their original semantics; they cannot combine both inputs.
func (s Service) ResolveQueryInput(ctx context.Context, a Actor, legacy []uuid.UUID, scope *QueryScope, signal string) (observationscope.Snapshot, []string, error) {
	if scope != nil {
		if legacy != nil {
			return observationscope.Snapshot{}, nil, ErrQueryInvalid
		}
		return s.ResolveStructuredQueryScope(ctx, a, *scope, signal)
	}
	if len(legacy) == 0 {
		return observationscope.Snapshot{}, nil, ErrQueryInvalid
	}
	return s.ResolveQueryScope(ctx, a, legacy, signal)
}
