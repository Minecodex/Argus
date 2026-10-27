package dashboard

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type CatalogInput struct {
	SourceBinding   SourceBinding             `json:"source_binding"`
	Signal          string                    `json:"signal"`
	Kind            string                    `json:"kind"`
	Metric          string                    `json:"metric,omitempty"`
	Field           string                    `json:"field,omitempty"`
	Search          string                    `json:"search,omitempty"`
	Cursor          string                    `json:"cursor,omitempty"`
	SelectedValues  []string                  `json:"selected_values"`
	Filters         []telemetry.CatalogFilter `json:"filters"`
	ResourceIDs     []uuid.UUID               `json:"resource_ids"`
	ResourceMode    string                    `json:"resource_mode,omitempty"`
	ResourceTypes   []string                  `json:"resource_types,omitempty"`
	ApplicationMode string                    `json:"application_mode,omitempty"`
	ApplicationIDs  []uuid.UUID               `json:"application_ids,omitempty"`
	From            time.Time                 `json:"from"`
	To              time.Time                 `json:"to"`
	Limit           int                       `json:"limit"`
}

func (runtime Runtime) Catalog(ctx context.Context, actor Actor, input CatalogInput) (telemetry.DataCatalogResult, error) {
	runtime = runtime.metered(actor)
	empty := telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{}, Fields: []telemetry.CatalogField{}, Values: []string{}, Membership: map[string]bool{}, Complete: true}
	if runtime.Backend == nil {
		return empty, ErrUnavailable
	}
	if err := authorize(ctx, runtime.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return empty, err
	}
	if !input.To.After(input.From) || input.To.Sub(input.From) > 7*24*time.Hour || len(input.Metric) > 256 || len(input.Field) > 256 || len(input.Search) > 256 || input.Limit > 1000 || input.Limit < 1 || len(input.SelectedValues) > 200 {
		return empty, ErrInvalid
	}
	if input.Kind != "metrics" && input.Kind != "values" && input.Kind != "fields" || input.Kind == "metrics" && input.Signal != "metrics" || input.Kind == "values" && input.Field == "" {
		return empty, ErrInvalid
	}
	if input.Kind == "values" {
		if err := telemetry.ValidateCatalogDefinition(input.Signal, input.Field, input.Filters); err != nil {
			return empty, ErrInvalid
		}
	}
	if input.Kind != "values" && len(input.SelectedValues) > 0 || telemetry.ValidateCatalogFilters(input.Signal, input.Filters) != nil {
		return empty, ErrInvalid
	}
	snapshot, err := resolveExecutionScope(ctx, runtime.Store.Queries, actor, ExecutionInput{ResourceIDs: input.ResourceIDs, ResourceMode: input.ResourceMode, ResourceTypes: input.ResourceTypes, ApplicationIDs: input.ApplicationIDs, ApplicationMode: input.ApplicationMode})
	if err != nil {
		return empty, err
	}
	ids := []uuid.UUID{}
	for _, resource := range snapshot.Resources {
		ids = append(ids, resource.ID)
	}
	sources, err := resolveSources(ctx, runtime.Store.Queries, actor, input.SourceBinding, input.Signal, snapshot.Refs(), snapshot.Selection.Applications)
	if err != nil {
		return empty, err
	}
	if len(sources) == 0 {
		for _, value := range input.SelectedValues {
			empty.Membership[value] = false
		}
		return empty, nil
	}
	keys := []string{}
	for _, source := range sources {
		keys = append(keys, source.Key())
	}
	refs, err := dataResourcesForSources(sources)
	if err != nil {
		return empty, err
	}
	data, err := runtime.Backend.DiscoverData(ctx, telemetry.DataCatalogRequest{ScopeSignature: snapshot.Signature, Resources: refs, SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, EnterpriseID: actor.EnterpriseID, ResourceIDs: resourceref.IDs(refs), SourceKeys: keys, AuthorizationVersion: actor.AuthorizationVersion, Signal: input.Signal, Kind: input.Kind, Metric: input.Metric, Field: input.Field, Search: input.Search, Cursor: input.Cursor, SelectedValues: input.SelectedValues, Filters: input.Filters, From: input.From, To: input.To, Limit: input.Limit, Budget: queryengine.Budget{MaxScanBytes: telemetry.DefaultMaxScanBytes, MaxRows: input.Limit + len(input.SelectedValues), MaxResultBytes: 8 << 20, Timeout: telemetry.DefaultTimeout}})
	if err != nil {
		return empty, err
	}
	if err := authorize(ctx, runtime.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return empty, err
	}
	if _, err := resolveResources(ctx, runtime.Store.Queries, actor, ids); err != nil {
		return empty, err
	}
	runtime.scopeApplications = snapshot.Selection.Applications
	if err := runtime.checkQueryScope(ctx, actor, queryScope{Resources: snapshot.Refs(), Sources: sources}, input.Signal); err != nil {
		return empty, err
	}
	return data, nil
}
