package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/sourcekey"
)

type capabilityInput struct {
	Resources []resourceref.Ref `json:"resources"`
	Signals   []string          `json:"signals,omitempty"`
	Sections  []string          `json:"sections,omitempty"`
	Cursor    string            `json:"cursor,omitempty"`
}
type capabilitySection struct {
	Status          string `json:"status"`
	Items           any    `json:"items"`
	Complete        bool   `json:"complete"`
	NextCursor      string `json:"next_cursor,omitempty"`
	Coverage        string `json:"coverage,omitempty"`
	ScopeNote       string `json:"scope_note,omitempty"`
	EntryKind       string `json:"entry_kind,omitempty"`
	PageEntryCount  *int   `json:"page_entry_count,omitempty"`
	PageSourceCount *int   `json:"page_source_count,omitempty"`
}

type capabilityPosition struct {
	Position string `json:"position"`
	Revision string `json:"revision"`
}

func readCapabilityPosition(cursor, revision string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	var value capabilityPosition
	if json.Unmarshal([]byte(cursor), &value) != nil || value.Position == "" || value.Revision != revision {
		return "", monitoring.ErrConflict
	}
	return value.Position, nil
}
func writeCapabilityPosition(position, revision string) string {
	raw, _ := json.Marshal(capabilityPosition{position, revision})
	return string(raw)
}

func (t ObservationTools) capabilities(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
	in, err := domainInput[capabilityInput](c)
	if err != nil {
		return nil, err
	}
	refs, err := resourceref.Normalize(in.Resources)
	if err != nil || len(refs) == 0 || len(refs) > 20 || len(refs) != len(in.Resources) {
		return nil, monitoring.ErrInvalid
	}
	if len(in.Sections) == 0 {
		in.Sections = []string{"collection_state", "sources", "dashboards", "next_tools"}
	}
	if in.Cursor != "" && (len(refs) != 1 || len(in.Sections) != 1) {
		return nil, monitoring.ErrInvalid
	}
	resolver := observationscope.Resolver{Queries: t.Base.Store.Queries}
	types := []string{}
	for _, ref := range refs {
		if !slices.Contains(types, ref.Type) {
			types = append(types, ref.Type)
		}
	}
	snapshot, err := resolver.Resolve(ctx, a, observationscope.Selection{Mode: observationscope.Explicit, IDs: resourceref.IDs(refs), Types: types, Signal: "catalog"})
	if err != nil {
		return nil, err
	}
	if !slices.Equal(snapshot.Refs(), refs) {
		return nil, monitoring.ErrDenied
	}
	out := map[string]any{"items": []any{}, "complete": true, "sampling_performed": false, "data_state": "not_checked",
		"data_state_reason": "This capability request reads authorized configuration and registration metadata only; it did not query telemetry samples. sampling_performed describes this request's activity, not whether a target is collecting or has historical data. Do not derive it from enabled, stopped, health or freshness states."}
	items := []any{}
	for _, resource := range snapshot.Resources {
		ref := resourceref.Ref{Type: resource.Kind, ID: resource.ID}
		sections := map[string]capabilitySection{}
		for _, section := range in.Sections {
			binding := pagination.Binding{Audience: "observe_capabilities/" + section, EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter([]any{ref, in.Signals, resource.Version, resource.NamespaceUID}), Sort: "stable"}
			signer := pagination.Signer{Key: t.CursorKey}
			after := ""
			if in.Cursor != "" {
				pos, e := signer.Decode(in.Cursor, binding)
				if e != nil {
					return nil, e
				}
				after = pos.ID
			}
			part, next, e := t.capabilitySection(ctx, a, ref, in.Signals, section, after)
			if e != nil {
				if in.Cursor != "" {
					return nil, e
				}
				if errors.Is(e, authorization.ErrAccessDenied) || errors.Is(e, dashboard.ErrDenied) {
					part = capabilitySection{Status: "not_permitted", Items: []any{}, Complete: false}
				} else if errors.Is(e, monitoring.ErrInvalid) || errors.Is(e, pagination.ErrInvalid) {
					return nil, e
				} else {
					part = capabilitySection{Status: "unavailable", Items: []any{}, Complete: false}
				}
			}
			if next != "" {
				part.NextCursor, err = signer.Encode(binding, pagination.Position{ID: next, Time: time.Now().UTC()})
				if err != nil {
					return nil, err
				}
			}
			if !part.Complete {
				out["complete"] = false
			}
			sections[section] = part
		}
		items = append(items, map[string]any{"ref": ref, "id": ref.ID, "type": ref.Type, "name": resource.Name, "environment": resource.Environment, "sections": sections})
	}
	current, err := resolver.Resolve(ctx, a, snapshot.Selection)
	if err != nil {
		return nil, err
	}
	if current.Signature != snapshot.Signature {
		return nil, monitoring.ErrDenied
	}
	out["items"] = items
	return out, nil
}

func (t ObservationTools) capabilitySection(ctx context.Context, a authorization.Actor, ref resourceref.Ref, signals []string, section, after string) (capabilitySection, string, error) {
	part := capabilitySection{Status: "available", Items: []any{}, Complete: true}
	switch section {
	case "collection_state":
		part.Coverage = "collection_instances_only"
		part.ScopeNote = "This section lists authorized collection instances only. It does not inspect application SDK ingest endpoints or all signal sources. Empty collection instances do not prove an application has no monitoring or historical telemetry; inspect authorized sources/endpoints separately before making that claim."
		if err := authorization.Authorize(ctx, t.Base.Store.Queries, a, "monitoring.read"); err != nil {
			return part, "", err
		}
		revision, err := t.Base.Store.Queries.ObservationCollectionRevision(ctx, db.ObservationCollectionRevisionParams{EnterpriseID: a.EnterpriseID, TargetType: ref.Type, TargetID: ref.ID})
		if err != nil {
			return part, "", err
		}
		after, err = readCapabilityPosition(after, revision)
		if err != nil {
			return part, "", err
		}
		id := uuid.Nil
		if after != "" {
			id, err = uuid.Parse(after)
			if err != nil {
				return part, "", monitoring.ErrInvalid
			}
		}
		states, more, err := t.Monitoring.CollectionStates(ctx, a, monitoring.ResourceRef{Kind: ref.Type, ID: ref.ID}, id, 25)
		if err != nil {
			return part, "", err
		}
		part.Items = states
		part.Complete = !more
		if more {
			return part, writeCapabilityPosition(states[len(states)-1].ID.String(), revision), nil
		}
	case "sources":
		part.EntryKind = "source_version_selectors"
		part.ScopeNote = "Items are source-version query selectors, not independent source counts. Multiple config revisions can share the same source_id. page_entry_count and page_source_count cover this page only; preserve each full source_key for queries, including its revision/Namespace facet. current_installation means current Collector generation and not uninstalled; it does not prove latest revision, SDK endpoint enabled, receiving or health. Check authorized endpoint metadata for SDK enabled state."
		sources, err := (observationscope.Resolver{Queries: t.Base.Store.Queries}).Sources(ctx, a, observationscope.SourceSelection{Resources: []resourceref.Ref{ref}, Signal: "catalog"})
		if err != nil {
			return part, "", err
		}
		after, err = readCapabilityPosition(after, sources.Signature)
		if err != nil {
			return part, "", err
		}
		rows := []any{}
		for _, key := range sources.Keys {
			id, revision, _, valid := sourcekey.Parse(key)
			if !valid {
				continue
			}
			for _, row := range sources.Rows {
				if row.ID == id && row.ConfigRevision == revision && (len(signals) == 0 || slices.ContainsFunc(signals, func(s string) bool { return slices.Contains(row.Signals, s) })) {
					rows = append(rows, map[string]any{"source_id": id.String(), "source_key": key, "entry_kind": "source_version_selector", "source_type": row.SourceType, "config_revision": row.ConfigRevision, "signals": row.Signals, "current_installation": row.CurrentInstallation, "installation_scope": "collector_generation_only", "data_state": "not_checked", "sampling_performed": false, "data_state_reason": "This discovery does not query telemetry samples. Registration/current Collector generation is not endpoint enabled, receiving, latest revision or health; historical availability remains unchecked."})
					break
				}
			}
		}
		start := 0
		if after != "" {
			start, err = strconv.Atoi(after)
			if err != nil || start < 0 || start > len(rows) {
				return part, "", monitoring.ErrInvalid
			}
		}
		end := min(start+25, len(rows))
		part.Items = rows[start:end]
		pageEntries := end - start
		pageSources := map[string]bool{}
		for _, raw := range rows[start:end] {
			pageSources[raw.(map[string]any)["source_id"].(string)] = true
		}
		sourceCount := len(pageSources)
		part.PageEntryCount, part.PageSourceCount = &pageEntries, &sourceCount
		part.Complete = end == len(rows)
		if !part.Complete {
			return part, writeCapabilityPosition(strconv.Itoa(end), sources.Signature), nil
		}
	case "dashboards":
		part.ScopeNote = "These are navigation links evaluated separately for this single resource. They are not aggregate applicability for the requested resource set. For dashboard type/signal/integration filters and combined-resource applicability, search/describe dashboard.list and pass the explicit resources and filters. Empty links for one resource are not a returned partial suitability for several resources."
		offset := 0
		var err error
		var prior capabilityPosition
		if after != "" {
			if json.Unmarshal([]byte(after), &prior) != nil {
				return part, "", monitoring.ErrInvalid
			}
			offset, err = strconv.Atoi(prior.Position)
			if err != nil {
				return part, "", monitoring.ErrInvalid
			}
		}
		page, err := t.Dashboard.Discover(ctx, a, dashboard.DiscoveryInput{Resources: []resourceref.Ref{ref}, Signals: signals, Offset: offset, Limit: 25})
		if err != nil {
			return part, "", err
		}
		if after != "" && prior.Revision != page.CatalogRevision {
			return part, "", monitoring.ErrConflict
		}
		part.Items = page.Items
		part.Complete = page.Complete
		if page.HasMore {
			return part, writeCapabilityPosition(strconv.Itoa(page.NextOffset), page.CatalogRevision), nil
		}
	case "next_tools":
		if after != "" {
			return part, "", monitoring.ErrInvalid
		}
		refs := []map[string]any{{"category": "observe", "name": "catalog.query", "purpose": "Discover actual fields and query recipes"}, {"category": "dashboard", "name": "list", "purpose": "Filter published dashboards by type/signal/integration and evaluate applicability across the explicit resource set; capability links are per-resource only"}}
		if ref.Type == "application" {
			refs = append([]map[string]any{{"category": "application", "name": "endpoint.list", "purpose": "Check configured SDK ingress protocol, signals and current enabled state; source registration/current_installation is not this state"}}, refs...)
		}
		for _, signal := range []string{"metrics", "logs", "traces"} {
			if len(signals) > 0 && !slices.Contains(signals, signal) {
				continue
			}
			category := map[string]string{"metrics": "metric", "logs": "log", "traces": "trace"}[signal]
			refs = append(refs, map[string]any{"category": category, "name": "query", "purpose": "Query authorized " + signal})
		}
		part.Items = refs
	default:
		return part, "", monitoring.ErrInvalid
	}
	return part, "", nil
}
