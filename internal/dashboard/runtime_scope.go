package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type ResourceScope = resourceref.Ref
type ResolvedSource struct {
	ScopeRelations      map[string]int64 `json:"scope_relations,omitempty"`
	ScopeResourceIDs    []uuid.UUID      `json:"scope_resource_ids,omitempty"`
	ScopeSignature      string           `json:"scope_signature,omitempty"`
	ResourceType        string           `json:"resource_type,omitempty"`
	ID                  uuid.UUID        `json:"id"`
	Revision            int64            `json:"revision"`
	ResourceID          uuid.UUID        `json:"resource_id"`
	Generation          uuid.UUID        `json:"generation"`
	Type                string           `json:"type"`
	CapabilityVersion   string           `json:"capability_version"`
	CurrentInstallation *bool            `json:"current_installation,omitempty"`
	RegisteredAt        *time.Time       `json:"registered_at,omitempty"`
}

func (source ResolvedSource) Key() string {
	return source.ID.String() + ":" + strconv.FormatInt(source.Revision, 10)
}

var sourceSignals = func() map[string][]string {
	result := map[string][]string{
		"otlp": {"metrics", "logs", "traces"}, "hostmetrics": {"metrics"}, "prometheus": {"metrics"}, "kubeletstats": {"metrics"}, "k8s_cluster": {"metrics"},
		"filelog": {"logs"}, "journald": {"logs"}, "windowseventlog": {"logs"},
		"skywalking": {"traces"}, "jaeger": {"traces"},
	}
	for _, entry := range monitoringcatalog.All() {
		if entry.Component != "" && entry.Family != "ingestion" {
			result[entry.Component] = entry.Signals
		}
	}
	for _, entry := range builtincollection.All() {
		result[entry.SourceType] = []string{"metrics"}
	}
	return result
}()

func resolveResources(ctx context.Context, q *db.Queries, actor Actor, requested []uuid.UUID) ([]ResourceScope, error) {
	mode := observationscope.Explicit
	if requested == nil {
		mode = observationscope.AllAuthorized
	}
	snapshot, err := (observationscope.Resolver{Queries: q, Permission: "telemetry.dashboard.read"}).Resolve(ctx, actor, observationscope.Selection{Mode: mode, IDs: requested, Signal: "catalog"})
	if errors.Is(err, observationscope.ErrDenied) {
		return nil, ErrDenied
	}
	if errors.Is(err, observationscope.ErrInvalid) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	result := make([]ResourceScope, 0, len(snapshot.Resources))
	for _, item := range snapshot.Resources {
		result = append(result, ResourceScope{ID: item.ID, Type: item.Kind})
	}
	return result, nil
}

func validateTypedResources(ctx context.Context, q *db.Queries, actor Actor, resources []ResourceScope) error {
	_, err := typedResourceSnapshot(ctx, q, actor, resources)
	return err
}

func typedResourceSnapshot(ctx context.Context, q *db.Queries, actor Actor, resources []ResourceScope) (observationscope.Snapshot, error) {
	expected, err := resourceref.Normalize(resources)
	if err != nil {
		return observationscope.Snapshot{}, ErrInvalid
	}
	types := []string{}
	for _, ref := range expected {
		if !slices.Contains(types, ref.Type) {
			types = append(types, ref.Type)
		}
	}
	current, err := (observationscope.Resolver{Queries: q, Permission: "telemetry.dashboard.read"}).Resolve(ctx, actor, observationscope.Selection{Mode: observationscope.Explicit, IDs: resourceref.IDs(expected), Types: types, Signal: "catalog"})
	if errors.Is(err, observationscope.ErrDenied) {
		return current, ErrDenied
	}
	if errors.Is(err, observationscope.ErrInvalid) {
		return current, ErrInvalid
	}
	if err != nil {
		return current, err
	}
	if !slices.Equal(current.Refs(), expected) {
		return current, ErrDenied
	}
	return current, nil
}

func derivedScopeIdentity(ctx context.Context, q *db.Queries, actor Actor, resources []ResourceScope, parent Execution) (string, ScopeVersions, error) {
	snapshot, err := typedResourceSnapshot(ctx, q, actor, resources)
	if err != nil {
		return "", ScopeVersions{}, err
	}
	versions := ScopeVersions{Resources: map[string]int64{}, Relations: map[string]int64{}}
	for _, resource := range snapshot.Resources {
		versions.Resources[resource.Kind+":"+resource.ID.String()] = resource.Version
	}
	for id, version := range parent.ScopeVersions.Relations {
		versions.Relations[id] = version
	}
	material, _ := json.Marshal(struct {
		Parent, Current string
		Versions        ScopeVersions
	}{parent.ScopeSignature, snapshot.Signature, versions})
	return queryHash(material), versions, nil
}

func resolveExecutionScope(ctx context.Context, q *db.Queries, actor Actor, input ExecutionInput) (observationscope.Snapshot, error) {
	mode := input.ResourceMode
	if mode == "" {
		mode = observationscope.AllAuthorized
		if len(input.ResourceIDs) > 0 {
			mode = observationscope.Explicit
		}
	}
	selection := observationscope.Selection{Mode: mode, IDs: input.ResourceIDs, Types: input.ResourceTypes, Signal: "catalog"}
	if input.ApplicationMode != "" || len(input.ApplicationIDs) > 0 {
		applicationMode := input.ApplicationMode
		if applicationMode == "" {
			applicationMode = observationscope.Explicit
		}
		selection.Applications = &observationscope.ApplicationSelection{Mode: applicationMode, IDs: input.ApplicationIDs}
	}
	snapshot, err := (observationscope.Resolver{Queries: q, Permission: "telemetry.dashboard.read"}).Resolve(ctx, actor, selection)
	if errors.Is(err, observationscope.ErrDenied) {
		return snapshot, ErrDenied
	}
	if errors.Is(err, observationscope.ErrInvalid) {
		return snapshot, ErrInvalid
	}
	return snapshot, err
}

func scopeResources(snapshot observationscope.Snapshot) []ResourceScope {
	result := make([]ResourceScope, 0, len(snapshot.Resources))
	for _, item := range snapshot.Resources {
		result = append(result, ResourceScope{ID: item.ID, Type: item.Kind})
	}
	return result
}

func resolveSources(ctx context.Context, q *db.Queries, actor Actor, binding SourceBinding, signal string, resources []ResourceScope, applications ...*observationscope.ApplicationSelection) ([]ResolvedSource, error) {
	if binding.CapabilityVersion != "v1" || !slices.Contains(sourceSignals[binding.SourceType], signal) {
		return nil, ErrInvalid
	}
	if len(resources) == 0 {
		return []ResolvedSource{}, nil
	}
	refs, err := resourceref.Normalize(resources)
	if err != nil {
		return nil, ErrInvalid
	}
	var appSelection *observationscope.ApplicationSelection
	if len(applications) > 0 {
		appSelection = applications[0]
	}
	scoped, err := (observationscope.Resolver{Queries: q, Permission: "telemetry.dashboard.read"}).Sources(ctx, actor, observationscope.SourceSelection{Resources: refs, Signal: signal, SourceType: binding.SourceType, Applications: appSelection})
	if err != nil {
		if errors.Is(err, observationscope.ErrDenied) {
			return nil, ErrDenied
		}
		if errors.Is(err, observationscope.ErrInvalid) {
			return nil, ErrInvalid
		}
		return nil, err
	}
	result := []ResolvedSource{}
	for _, row := range scoped.Rows {
		if row.CapabilityVersion != binding.CapabilityVersion {
			return nil, ErrInvalid
		}
		current, registered := row.CurrentInstallation, row.CreatedAt.Time
		key := row.ID.String() + ":" + strconv.FormatInt(row.ConfigRevision, 10)
		result = append(result, ResolvedSource{ID: row.ID, Revision: row.ConfigRevision, ResourceID: row.ResourceID, ResourceType: row.ResourceType, Generation: row.Generation, Type: row.SourceType, CapabilityVersion: row.CapabilityVersion, CurrentInstallation: &current, RegisteredAt: &registered, ScopeResourceIDs: row.ScopeResourceIds, ScopeSignature: scoped.Signature, ScopeRelations: scoped.SourceRelations[key]})
	}
	return result, nil
}

func dataResourcesForSources(sources []ResolvedSource) ([]ResourceScope, error) {
	refs := []ResourceScope{}
	for _, source := range sources {
		ref := ResourceScope{Type: source.ResourceType, ID: source.ResourceID}
		if !resourceref.ValidTelemetryRoot(ref.Type) || ref.ID == uuid.Nil {
			return nil, ErrInvalid
		}
		if !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	refs, err := resourceref.Normalize(refs)
	if err != nil {
		return nil, ErrInvalid
	}
	return refs, nil
}
