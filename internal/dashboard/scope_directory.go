package dashboard

import (
	"context"
	"errors"
	"github.com/kakj-go/Argus/internal/observationscope"
)

type ScopeDirectoryInput = observationscope.DirectoryInput
type ScopeDirectoryItem = observationscope.DirectoryItem
type ScopeDirectoryPage = observationscope.DirectoryPage

func (s Service) ScopeDirectory(ctx context.Context, a Actor, in ScopeDirectoryInput) (ScopeDirectoryPage, error) {
	if s.Store == nil {
		return ScopeDirectoryPage{Items: []ScopeDirectoryItem{}}, ErrUnavailable
	}
	result, err := (observationscope.Resolver{Queries: s.Store.Queries, Permission: "telemetry.dashboard.read"}).ResourceDirectory(ctx, a, s.Actions.Key, in)
	if errors.Is(err, observationscope.ErrInvalid) {
		return result, ErrInvalid
	}
	return result, err
}
