package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type DiscoveryInput struct {
	Search        string            `json:"search,omitempty"`
	Offset        int               `json:"offset,omitempty"`
	Limit         int               `json:"limit,omitempty"`
	ResourceTypes []string          `json:"resource_types,omitempty"`
	Resources     []resourceref.Ref `json:"resources,omitempty"`
	Integrations  []string          `json:"integrations,omitempty"`
	Signals       []string          `json:"signals,omitempty"`
	ManagedOnly   bool              `json:"managed_only,omitempty"`
}
type DiscoveryItem struct {
	RevisionID        uuid.UUID `json:"revision_id"`
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	TemplateKey       string    `json:"managed_template_key,omitempty"`
	TemplateVersion   int64     `json:"template_version,omitempty"`
	ResourceTypes     []string  `json:"applicable_resource_types"`
	Integrations      []string  `json:"integrations"`
	Signals           []string  `json:"signals"`
	Suitability       string    `json:"suitability"`
	MatchReasons      []string  `json:"match_reasons"`
	DataState         string    `json:"data_state"`
	SamplingPerformed bool      `json:"sampling_performed"`
	DataStateReason   string    `json:"data_state_reason"`
}
type DiscoveryPage struct {
	CatalogRevision   string          `json:"catalog_revision"`
	Items             []DiscoveryItem `json:"items"`
	HasMore           bool            `json:"has_more"`
	NextOffset        int             `json:"next_offset"`
	Complete          bool            `json:"complete"`
	SelectionRequired bool            `json:"selection_required"`
	SelectionPolicy   string          `json:"selection_policy"`
}

func uniqueString(values []string, value string) []string {
	if value != "" && !slices.Contains(values, value) {
		values = append(values, value)
	}
	return values
}
func sourceIntegrationKeys(source string) []string {
	out := []string{}
	for _, e := range monitoringcatalog.All() {
		if e.Component == source {
			out = uniqueString(out, e.Key)
		}
	}
	for _, e := range builtincollection.All() {
		if e.SourceType == source {
			out = uniqueString(out, e.Key)
		}
	}
	return out
}
func intersects(a, b []string) bool {
	return slices.ContainsFunc(a, func(x string) bool { return slices.Contains(b, x) })
}

func (s Service) Discover(ctx context.Context, a Actor, in DiscoveryInput) (DiscoveryPage, error) {
	out := DiscoveryPage{Items: []DiscoveryItem{}, Complete: true, SelectionPolicy: "automatic"}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 50 || in.Offset < 0 || len(in.Search) > 256 || len(in.Resources) > 1000 || len(in.ResourceTypes) > 5 || len(in.Signals) > 3 || len(in.Integrations) > 64 {
		return out, ErrInvalid
	}
	for _, kind := range in.ResourceTypes {
		if !resourceref.ValidType(kind) {
			return out, ErrInvalid
		}
	}
	for _, signal := range in.Signals {
		if !slices.Contains([]string{"metrics", "logs", "traces"}, signal) {
			return out, ErrInvalid
		}
	}
	requested := map[resourceref.Ref][]string{}
	if len(in.Resources) > 0 {
		refs, err := resourceref.Normalize(in.Resources)
		if err != nil || len(refs) != len(in.Resources) {
			return out, ErrInvalid
		}
		types := []string{}
		for _, ref := range refs {
			types = uniqueString(types, ref.Type)
		}
		resolver := observationscope.Resolver{Queries: s.Store.Queries, Permission: "telemetry.dashboard.read"}
		sources, err := resolver.Sources(ctx, a, observationscope.SourceSelection{Resources: refs, Signal: "catalog"})
		if err != nil {
			return out, err
		}
		for _, ref := range refs {
			requested[ref] = []string{}
		}
		for _, row := range sources.Rows {
			for _, ref := range refs {
				if slices.Contains(row.ScopeResourceIds, ref.ID) {
					for _, key := range sourceIntegrationKeys(row.SourceType) {
						requested[ref] = uniqueString(requested[ref], key)
					}
				}
			}
		}
		for _, ref := range refs {
			if ref.Type == "monitoring_target" {
				target, e := s.Store.Queries.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: a.EnterpriseID, ID: ref.ID})
				if e != nil {
					return out, e
				}
				requested[ref] = uniqueString(requested[ref], target.Engine)
			}
		}
	}
	rows, err := s.List(ctx, a)
	if err != nil {
		return out, err
	}
	items := []DiscoveryItem{}
	for _, row := range rows {
		if row.Lifecycle != "active" || !row.ActiveRevisionID.Valid || !strings.Contains(strings.ToLower(row.Name+" "+row.Description), strings.ToLower(in.Search)) {
			continue
		}
		instance, e := s.Store.Queries.GetManagedDashboardByID(ctx, db.GetManagedDashboardByIDParams{EnterpriseID: a.EnterpriseID, DashboardID: row.ID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return out, e
		}
		managed := e == nil
		if in.ManagedOnly && !managed {
			continue
		}
		_, rev, e := s.Get(ctx, a, row.ID)
		if e != nil {
			return out, e
		}
		var spec Spec
		if json.Unmarshal(rev.Spec, &spec) != nil {
			return out, ErrInvalid
		}
		item := DiscoveryItem{ID: row.ID, Name: row.Name, Description: row.Description, ResourceTypes: []string{}, Integrations: []string{}, Signals: []string{}, Suitability: "applicable", MatchReasons: []string{}, DataState: "not_checked", DataStateReason: "Listing checks template applicability without executing panels. It does not assert that data is absent or stale; sample availability requires an actual query."}
		if managed {
			item.TemplateKey = instance.TemplateKey
			item.TemplateVersion = instance.AppliedVersion
		}
		item.RevisionID = rev.ID
		for _, p := range spec.Panels {
			item.Signals = uniqueString(item.Signals, p.Signal)
			for _, kind := range p.ApplicableResourceTypes {
				item.ResourceTypes = uniqueString(item.ResourceTypes, kind)
			}
			for _, key := range sourceIntegrationKeys(p.SourceBinding.SourceType) {
				item.Integrations = uniqueString(item.Integrations, key)
			}
		}
		if len(in.ResourceTypes) > 0 && !intersects(in.ResourceTypes, item.ResourceTypes) {
			continue
		}
		if len(in.Signals) > 0 && !intersects(in.Signals, item.Signals) {
			continue
		}
		if len(in.Integrations) > 0 && !intersects(in.Integrations, item.Integrations) {
			continue
		}
		matched := 0
		for ref, keys := range requested {
			kind := ref.Type
			if kind == "kubernetes_namespace" {
				kind = "kubernetes_cluster"
			}
			if slices.Contains(item.ResourceTypes, kind) && (len(item.Integrations) == 0 || intersects(keys, item.Integrations)) {
				matched++
			}
		}
		if len(requested) > 0 {
			if matched == 0 {
				continue
			}
			if matched < len(requested) {
				item.Suitability = "partial"
			}
			item.MatchReasons = append(item.MatchReasons, "authorized_resource_type_and_source")
		}
		if len(in.Integrations) > 0 {
			item.MatchReasons = append(item.MatchReasons, "integration")
		}
		if len(in.Signals) > 0 {
			item.MatchReasons = append(item.MatchReasons, "signal")
		}
		items = append(items, item)
	}
	raw, _ := json.Marshal(items)
	digest := sha256.Sum256(raw)
	out.CatalogRevision = hex.EncodeToString(digest[:])
	start := min(in.Offset, len(items))
	end := min(start+in.Limit, len(items))
	out.Items = items[start:end]
	out.NextOffset = end
	out.HasMore = end < len(items)
	out.Complete = !out.HasMore
	return out, nil
}
