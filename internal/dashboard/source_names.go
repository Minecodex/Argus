package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type sourceNameVersion struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision"`
}

// Names decorate the response after the execution is signed. A telemetry scope
// may include sources reached through a namespace without granting control-plane
// access to the source's actual resource or its collection configuration.
func populateSourceNames(ctx context.Context, q *db.Queries, actor Actor, resources []ResourceScope, groups ...[]ResolvedSource) error {
	granted := make(map[ResourceScope]bool, len(resources))
	for _, resource := range resources {
		granted[resource] = true
	}
	versions := map[sourceNameVersion]bool{}
	includesApplication := false
	for _, sources := range groups {
		for i := range sources {
			source := &sources[i]
			source.Name = ""
			if granted[ResourceScope{Type: source.ResourceType, ID: source.ResourceID}] {
				versions[sourceNameVersion{source.ID, source.Revision}] = true
				includesApplication = includesApplication || source.ResourceType == "application"
			}
		}
	}
	if len(versions) == 0 {
		return nil
	}
	collectionErr := authorize(ctx, q, actor, "monitoring.read")
	if collectionErr != nil && !errors.Is(collectionErr, ErrDenied) {
		return collectionErr
	}
	readEndpoints := false
	if includesApplication {
		endpointErr := authorize(ctx, q, actor, "application.read")
		if endpointErr != nil && !errors.Is(endpointErr, ErrDenied) {
			return endpointErr
		}
		readEndpoints = endpointErr == nil
	}
	if collectionErr != nil && !readEndpoints {
		return nil
	}
	refs, err := json.Marshal(slices.Collect(maps.Keys(versions)))
	if err != nil {
		return err
	}
	rows, err := q.ListDashboardSourceNames(ctx, db.ListDashboardSourceNamesParams{
		EnterpriseID: actor.EnterpriseID, SourceRefs: refs, ReadCollections: collectionErr == nil, ReadEndpoints: readEndpoints,
	})
	if err != nil {
		return err
	}
	names := make(map[sourceNameVersion]string, len(rows))
	for _, row := range rows {
		names[sourceNameVersion{row.ID, row.ConfigRevision}] = row.Name
	}
	for _, sources := range groups {
		for i := range sources {
			sources[i].Name = names[sourceNameVersion{sources[i].ID, sources[i].Revision}]
		}
	}
	return nil
}
