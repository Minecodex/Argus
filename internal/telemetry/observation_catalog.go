package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/sourcekey"
)

type ObservationRecipe struct {
	ID             string   `json:"id"`
	Version        int64    `json:"version"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Expression     string   `json:"expression"`
	Unit           string   `json:"unit"`
	SourceType     string   `json:"source_type"`
	IntegrationKey string   `json:"integration_key"`
	Metrics        []string `json:"metrics"`
}
type ObservationMetric struct {
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Unit        string              `json:"unit"`
	Labels      []string            `json:"labels"`
	SourceType  string              `json:"source_type"`
	SourceKeys  []string            `json:"source_keys"`
	Description string              `json:"description"`
	Observed    bool                `json:"observed"`
	Recipes     []ObservationRecipe `json:"recipes"`
}
type ObservationSource struct {
	SourceType        string   `json:"source_type"`
	CapabilityVersion string   `json:"capability_version"`
	Keys              []string `json:"keys"`
	IntegrationKeys   []string `json:"integration_keys"`
}
type ObservationCatalogInput struct {
	Scope          QueryScope      `json:"scope"`
	Signal         string          `json:"signal"`
	Kind           string          `json:"kind"`
	Metric         string          `json:"metric,omitempty"`
	Field          string          `json:"field,omitempty"`
	Search         string          `json:"search,omitempty"`
	Cursor         string          `json:"cursor,omitempty"`
	SelectedValues []string        `json:"selected_values,omitempty"`
	Filters        []CatalogFilter `json:"filters,omitempty"`
	From           time.Time       `json:"from"`
	To             time.Time       `json:"to"`
	Limit          int             `json:"limit"`
	ObservedOnly   *bool           `json:"observed_only,omitempty"`
}
type ObservationCatalogResult struct {
	Metrics    []ObservationMetric `json:"metrics"`
	Sources    []ObservationSource `json:"sources"`
	Fields     []CatalogField      `json:"fields"`
	Values     []string            `json:"values"`
	HasMore    bool                `json:"has_more"`
	Complete   bool                `json:"complete"`
	NextCursor string              `json:"next_cursor,omitempty"`
	Membership map[string]bool     `json:"selected_exists"`
	Warnings   []string            `json:"warnings"`
}
type observationCursor struct {
	Phase   string
	Backend string
	Index   int
}

func (s Service) ObservationCatalog(ctx context.Context, a Actor, in ObservationCatalogInput, recipes []ObservationRecipe) (ObservationCatalogResult, error) {
	out := ObservationCatalogResult{Metrics: []ObservationMetric{}, Sources: []ObservationSource{}, Fields: []CatalogField{}, Values: []string{}, Membership: map[string]bool{}, Warnings: []string{}, Complete: true}
	if !slices.Contains([]string{"metrics", "logs", "traces"}, in.Signal) || !slices.Contains([]string{"metrics", "fields", "values", "sources"}, in.Kind) || in.Kind == "metrics" && in.Signal != "metrics" || in.Kind == "values" && in.Field == "" || in.Limit < 1 || in.Limit > 100 || len(in.Search) > 256 || len(in.Metric) > 256 || len(in.Field) > 256 || len(in.Filters) > 32 || len(in.SelectedValues) > 200 || !in.To.After(in.From) || in.To.Sub(in.From) > 7*24*time.Hour {
		return out, ErrQueryInvalid
	}
	if ValidateCatalogFilters(in.Signal, in.Filters) != nil {
		return out, ErrQueryInvalid
	}
	if in.Kind == "values" && ValidateCatalogDefinition(in.Signal, in.Field, in.Filters) != nil {
		return out, ErrQueryInvalid
	}
	if in.Kind != "values" && len(in.SelectedValues) > 0 {
		return out, ErrQueryInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	snapshot, keys, err := s.ResolveStructuredQueryScope(ctx, a, in.Scope, in.Signal)
	if err != nil {
		return out, err
	}
	groups, err := s.observationSources(ctx, a, snapshot)
	if err != nil {
		return out, err
	}
	raw, _ := json.Marshal(recipes)
	digest := sha256.Sum256(raw)
	base := DataCatalogRequest{EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, Resources: snapshot.DataRefs(), ResourceIDs: snapshot.DataIDs(), SourceKeys: keys, ScopeSignature: snapshot.QuerySignature(), Signal: in.Signal, Kind: in.Kind, Metric: in.Metric, Field: in.Field, Search: in.Search, Filters: in.Filters, SelectedValues: in.SelectedValues, From: in.From, To: in.To, Limit: in.Limit, Budget: queryengine.Budget{MaxScanBytes: DefaultMaxScanBytes, MaxRows: in.Limit + len(in.SelectedValues), MaxResultBytes: 8 << 20, Timeout: DefaultTimeout}}
	cursorContext := base
	contextDigest := sha256.Sum256([]byte(snapshot.QuerySignature() + hex.EncodeToString(digest[:])))
	cursorContext.ScopeSignature = hex.EncodeToString(contextDigest[:])
	cursorContext.Kind = "observation_" + in.Kind + strconv.FormatBool(in.ObservedOnly == nil || *in.ObservedOnly)
	cursorContext.Cursor = in.Cursor
	position, err := readCatalogCursor(cursorContext)
	if err != nil {
		return out, err
	}
	page := observationCursor{Phase: "observed"}
	if position != "" && json.Unmarshal([]byte(position), &page) != nil {
		return out, ErrQueryInvalid
	}
	if page.Index < 0 || !slices.Contains([]string{"observed", "supported"}, page.Phase) {
		return out, ErrQueryInvalid
	}
	next := func() {
		encoded, _ := json.Marshal(page)
		out.NextCursor = writeCatalogCursor(cursorContext, string(encoded))
		out.HasMore = true
		out.Complete = false
	}
	if in.Kind == "sources" {
		if page.Index > len(groups) {
			return out, ErrQueryInvalid
		}
		end := min(page.Index+in.Limit, len(groups))
		out.Sources = groups[page.Index:end]
		if end < len(groups) {
			page.Index = end
			next()
		}
	} else if len(keys) > 0 {
		backend, ok := s.Engine.(DataCatalogBackend)
		if !ok {
			return out, ErrUnavailable
		}
		used := int64(0)
		discover := func(request DataCatalogRequest) (DataCatalogResult, error) {
			request.Budget.MaxScanBytes = DefaultMaxScanBytes - used
			if request.Budget.MaxScanBytes <= 0 {
				return DataCatalogResult{}, ErrQueryBudget
			}
			r, e := backend.DiscoverData(ctx, request)
			used += r.Meta.ScannedBytes
			if used > DefaultMaxScanBytes {
				return r, ErrQueryBudget
			}
			out.Warnings = append(out.Warnings, r.Meta.Warnings...)
			return r, e
		}
		if in.Kind != "metrics" {
			base.Cursor = page.Backend
			r, e := discover(base)
			if e != nil {
				return out, e
			}
			out.Fields, out.Values, out.Membership = r.Fields, r.Values, r.Membership
			out.Complete = r.Complete
			if r.HasMore {
				page.Backend = r.NextCursor
				next()
			}
		} else {
			if page.Phase == "observed" {
				base.Kind = "metric_variants"
				base.Cursor = page.Backend
				r, e := discover(base)
				if e != nil {
					return out, e
				}
				out.Complete = r.Complete
				for _, m := range r.Metrics {
					out.Metrics = append(out.Metrics, enrichObservedMetric(m, recipes, groups))
				}
				if r.HasMore {
					page.Backend = r.NextCursor
					next()
				} else {
					page.Phase = "supported"
					page.Backend = ""
					page.Index = 0
				}
			}
			if !out.HasMore && in.ObservedOnly != nil && !*in.ObservedOnly {
				candidates := supportedMetrics(recipes, groups, in.Search)
				if page.Index > len(candidates) {
					return out, ErrQueryInvalid
				}
				examined := 0
				for page.Index < len(candidates) && len(out.Metrics) < in.Limit && examined < 100 {
					candidate := candidates[page.Index]
					page.Index++
					examined++
					request := base
					request.Cursor = ""
					request.Kind = "metric_variants"
					request.Metric = candidate.Name
					request.Search = ""
					request.SourceKeys = candidate.SourceKeys
					request.Limit = 1
					r, e := discover(request)
					if e != nil {
						return out, e
					}
					// The observation phase has already enumerated existing variants. Only a
					// complete empty targeted lookup can establish that a definition is unseen.
					if len(r.Metrics) == 0 && r.Complete && !r.HasMore {
						out.Metrics = append(out.Metrics, candidate)
					}
				}
				if page.Index < len(candidates) {
					next()
				}
			}
		}
	}
	if err = s.ValidateResolvedQueryScope(ctx, a, snapshot); err != nil {
		return out, err
	}
	if in.Cursor != "" || in.Search != "" {
		out.Complete = false
	}
	return out, nil
}

func (s Service) observationSources(ctx context.Context, a Actor, snapshot observationscope.Snapshot) ([]ObservationSource, error) {
	out := []ObservationSource{}
	byType := map[string]int{}
	integrations := map[uuid.UUID]string{}
	if snapshot.Query == nil {
		return out, nil
	}
	for _, row := range snapshot.Query.Rows {
		index, ok := byType[row.SourceType]
		if !ok {
			index = len(out)
			byType[row.SourceType] = index
			out = append(out, ObservationSource{SourceType: row.SourceType, CapabilityVersion: "v1", Keys: []string{}, IntegrationKeys: []string{}})
		}
		for _, key := range snapshot.Query.Keys {
			id, _, _, valid := sourcekey.Parse(key)
			if valid && id == row.ID && !slices.Contains(out[index].Keys, key) {
				out[index].Keys = append(out[index].Keys, key)
			}
		}
		if row.CollectionID.Valid {
			integration, known := integrations[row.CollectionID.UUID]
			if !known {
				collection, err := s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: row.CollectionID.UUID})
				if err != nil {
					return out, err
				}
				integration = collection.IntegrationKey
				integrations[row.CollectionID.UUID] = integration
			}
			if !slices.Contains(out[index].IntegrationKeys, integration) {
				out[index].IntegrationKeys = append(out[index].IntegrationKeys, integration)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceType < out[j].SourceType })
	for i := range out {
		slices.Sort(out[i].Keys)
		slices.Sort(out[i].IntegrationKeys)
	}
	return out, nil
}

func enrichObservedMetric(m MetricDescriptor, recipes []ObservationRecipe, sources []ObservationSource) ObservationMetric {
	out := ObservationMetric{Name: m.Name, Type: m.Type, Unit: m.Unit, Labels: m.Labels, SourceType: m.SourceType, SourceKeys: m.SourceKeys, Observed: true, Recipes: []ObservationRecipe{}}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if out.SourceKeys == nil {
		out.SourceKeys = []string{}
	}
	for _, r := range recipes {
		if r.SourceType == m.SourceType && slices.Contains(r.Metrics, m.Name) {
			out.Recipes = append(out.Recipes, r)
			if out.Description == "" {
				out.Description = r.Description
			}
		}
	}
	return out
}

func supportedMetrics(recipes []ObservationRecipe, sources []ObservationSource, search string) []ObservationMetric {
	items := map[string]ObservationMetric{}
	for _, r := range recipes {
		for _, source := range sources {
			if source.SourceType != r.SourceType || !slices.Contains(source.IntegrationKeys, r.IntegrationKey) {
				continue
			}
			for _, name := range r.Metrics {
				if !strings.Contains(strings.ToLower(name+" "+r.Description+" "+r.Title), strings.ToLower(search)) {
					continue
				}
				key := fmt.Sprintf("%s\x00%s", source.SourceType, name)
				m, exists := items[key]
				if !exists {
					m = ObservationMetric{Name: name, Type: "unknown", Labels: []string{}, SourceType: source.SourceType, SourceKeys: source.Keys, Description: r.Description, Recipes: []ObservationRecipe{}}
				}
				m.Recipes = append(m.Recipes, r)
				items[key] = m
			}
		}
	}
	keys := []string{}
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []ObservationMetric{}
	for _, k := range keys {
		out = append(out, items[k])
	}
	return out
}
