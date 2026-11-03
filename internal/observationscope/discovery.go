package observationscope

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type DiscoveryInput struct {
	ResourceTypes []string `json:"resource_types,omitempty"`
	Families      []string `json:"families,omitempty"`
	Integrations  []string `json:"integrations,omitempty"`
	Search        string   `json:"search,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Cursor        string   `json:"cursor,omitempty"`
}
type DiscoveryItem struct {
	DirectoryItem
	Ref          resourceref.Ref `json:"ref"`
	Family       string          `json:"family,omitempty"`
	Integrations []string        `json:"integrations"`
}
type DiscoveryPage struct {
	Items      []DiscoveryItem `json:"items"`
	Complete   bool            `json:"complete"`
	HasMore    bool            `json:"has_more"`
	NextCursor string          `json:"next_cursor,omitempty"`
	ObservedAt time.Time       `json:"observed_at"`
}

func discoveryIntegrations(in DiscoveryInput) ([]string, bool, error) {
	known := map[string]string{}
	for _, e := range monitoringcatalog.All() {
		known[e.Key] = e.Family
	}
	for _, e := range builtincollection.All() {
		known[e.Key] = "infrastructure"
	}
	for _, family := range in.Families {
		if !slices.ContainsFunc(monitoringcatalog.All(), func(e monitoringcatalog.Entry) bool { return e.Family == family }) && family != "infrastructure" {
			return nil, false, ErrInvalid
		}
	}
	for _, key := range in.Integrations {
		if _, ok := known[key]; !ok {
			return nil, false, ErrInvalid
		}
	}
	result := []string{}
	for key, family := range known {
		if (len(in.Integrations) == 0 || slices.Contains(in.Integrations, key)) && (len(in.Families) == 0 || slices.Contains(in.Families, family)) {
			result = append(result, key)
		}
	}
	slices.Sort(result)
	return result, len(in.Integrations) > 0 || len(in.Families) > 0, nil
}

// Discover uses the same authoritative directory as portal scope selection.
// All filters execute in SQL before pagination; family is collection metadata,
// never an authorization type or a replacement for source identity.
func (r Resolver) Discover(ctx context.Context, a authorization.Actor, key []byte, in DiscoveryInput) (DiscoveryPage, error) {
	out := DiscoveryPage{Items: []DiscoveryItem{}, Complete: true, ObservedAt: time.Now().UTC()}
	if r.Queries == nil || len(in.ResourceTypes) > 5 || len(in.Families) > 32 || len(in.Integrations) > 64 || len(in.Search) > 200 || len(in.Cursor) > 4096 {
		return out, ErrInvalid
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return out, ErrInvalid
	}
	in.Search = strings.TrimSpace(in.Search)
	integrations, filtered, err := discoveryIntegrations(in)
	if err != nil {
		return out, err
	}
	types := slices.Clone(in.ResourceTypes)
	if len(types) == 0 {
		types = []string{"application", "host", "kubernetes_cluster", "kubernetes_namespace", "monitoring_target"}
	}
	slices.Sort(types)
	types = slices.Compact(types)
	refs := []resourceref.Ref{}
	readable := 0
	for _, kind := range types {
		permission := RootPermission(kind)
		if permission == "" {
			return out, ErrInvalid
		}
		if r.Permission != "" {
			permission = r.Permission
		}
		if err := authorization.Authorize(ctx, r.Queries, a, permission); err != nil {
			if len(in.ResourceTypes) == 0 && errors.Is(err, ErrDenied) {
				continue
			}
			return out, err
		}
		ids, err := authorization.AuthorizedResourceIDs(ctx, r.Queries, a, kind)
		readable++
		if err != nil {
			return out, err
		}
		for _, id := range ids {
			refs = append(refs, resourceref.Ref{Type: kind, ID: id})
		}
	}
	if readable == 0 {
		return out, ErrDenied
	}
	binding := pagination.Binding{Audience: "observation_discovery/v1/" + r.Permission, EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter([]any{types, in.Families, in.Integrations, in.Search}), Sort: "kind,id"}
	signer := pagination.Signer{Key: key}
	afterKind := ""
	afterID := uuid.Nil
	if in.Cursor != "" {
		position, e := signer.Decode(in.Cursor, binding)
		if e != nil {
			return out, e
		}
		parts := strings.SplitN(position.ID, "/", 2)
		if len(parts) != 2 || !resourceref.ValidType(parts[0]) {
			return out, ErrInvalid
		}
		afterKind = parts[0]
		afterID, e = uuid.Parse(parts[1])
		if e != nil {
			return out, ErrInvalid
		}
	}
	raw, _ := json.Marshal(refs)
	rows, err := r.Queries.ListObservationDiscoveryPage(ctx, db.ListObservationDiscoveryPageParams{EnterpriseID: a.EnterpriseID, AuthorizedRefs: raw, AfterKind: afterKind, AfterID: afterID, Search: in.Search, FilterIntegrations: filtered, Integrations: integrations, PageLimit: int32(in.Limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > in.Limit {
		last := rows[in.Limit-1]
		out.NextCursor, err = signer.Encode(binding, pagination.Position{ID: last.Kind + "/" + last.ID.String(), Time: out.ObservedAt})
		if err != nil {
			return out, err
		}
		out.HasMore = true
		out.Complete = false
		rows = rows[:in.Limit]
	}
	selected := []resourceref.Ref{}
	for _, row := range rows {
		ref := resourceref.Ref{Type: row.Kind, ID: row.ID}
		selected = append(selected, ref)
		item := DiscoveryItem{Ref: ref, Integrations: row.Integrations, DirectoryItem: DirectoryItem{ID: row.ID, Type: row.Kind, Name: row.Name, Environment: row.Environment, ParentName: row.ParentName, SourceUID: row.NamespaceUid}}
		if row.ParentID.Valid {
			id := row.ParentID.UUID
			item.ParentID = &id
		}
		if row.Integration != "" && !slices.Contains(item.Integrations, row.Integration) {
			item.Integrations = append(item.Integrations, row.Integration)
		}
		if entry, ok := monitoringcatalog.Lookup(row.Integration); ok {
			item.Family = entry.Family
		}
		if item.Integrations == nil {
			item.Integrations = []string{}
		}
		out.Items = append(out.Items, item)
	}
	if len(selected) > 0 {
		verified, e := r.Resolve(ctx, a, Selection{Mode: Explicit, IDs: resourceref.IDs(selected), Types: types, Signal: "catalog"})
		if e != nil {
			return DiscoveryPage{}, e
		}
		normalized, _ := resourceref.Normalize(selected)
		if !slices.Equal(normalized, verified.Refs()) {
			return DiscoveryPage{}, ErrDenied
		}
	}
	return out, nil
}
