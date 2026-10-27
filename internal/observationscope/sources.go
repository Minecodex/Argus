package observationscope

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/sourcekey"
)

type SourceSelection struct {
	Resources    []resourceref.Ref
	Applications *ApplicationSelection
	Signal       string
	SourceType   string
	// nil selects every authorized registered source. An explicit empty slice
	// selects none; frozen queries pass their exact source version keys here.
	Keys []string
}
type SourceResult struct {
	Input           SourceSelection
	Authority       Snapshot
	Rows            []db.ListObservationSourcesRow
	DataResources   []resourceref.Ref
	Keys            []string
	Signature       string
	Relations       map[string]int64
	SourceRelations map[string]map[string]int64
}
type sourceScope struct {
	Type                string     `json:"type"`
	ID                  uuid.UUID  `json:"id"`
	ClusterID           *uuid.UUID `json:"cluster_id,omitempty"`
	NamespaceUID        string     `json:"namespace_uid,omitempty"`
	AllowInfrastructure bool       `json:"allow_infrastructure"`
}

// Sources resolves selection authority before reading any data. Namespace
// authority remains distinct from the actual app/cluster identities in storage.
// No name-based inference or whole-cluster fallback is permitted.
func (r Resolver) Sources(ctx context.Context, a authorization.Actor, in SourceSelection) (SourceResult, error) {
	out := SourceResult{Input: in, Rows: []db.ListObservationSourcesRow{}, DataResources: []resourceref.Ref{}, Keys: []string{}, Relations: map[string]int64{}, SourceRelations: map[string]map[string]int64{}}
	refs, err := resourceref.Normalize(in.Resources)
	if err != nil || len(in.Keys) > 10000 || len(in.SourceType) > 128 {
		return out, ErrInvalid
	}
	types := []string{}
	for _, ref := range refs {
		if !slices.Contains(types, ref.Type) {
			types = append(types, ref.Type)
		}
	}
	out.Authority, err = r.Resolve(ctx, a, Selection{Mode: Explicit, IDs: resourceref.IDs(refs), Types: types, Applications: in.Applications, Signal: in.Signal})
	if err != nil {
		return out, err
	}
	if !slices.Equal(refs, out.Authority.Refs()) && in.Applications == nil {
		return out, ErrDenied
	}
	requested := map[string]bool{}
	for _, key := range in.Keys {
		_, _, namespace, ok := sourcekey.Parse(key)
		if !ok || requested[key] || namespace != "" && in.Signal != "metrics" && in.Signal != "catalog" {
			return out, ErrInvalid
		}
		requested[key] = true
	}
	if len(out.Authority.Resources) > 0 && (in.Keys == nil || len(in.Keys) > 0) {
		operational := false
		needsNamespacePolicy := false
		for _, ref := range refs {
			needsNamespacePolicy = needsNamespacePolicy || ref.Type == "kubernetes_namespace" || ref.Type == "kubernetes_cluster"
		}
		applications, targets := []uuid.UUID{}, []uuid.UUID{}
		if needsNamespacePolicy && (in.Signal == "logs" || in.Signal == "catalog") {
			if e := authorization.Authorize(ctx, r.Queries, a, "kubernetes.operational_logs.read"); e == nil {
				operational = true
			} else if !errors.Is(e, ErrDenied) {
				return out, e
			}
			if !operational {
				applications, err = r.sourceGrantIDs(ctx, a, "application")
				if err != nil {
					return out, err
				}
				targets, err = r.sourceGrantIDs(ctx, a, "monitoring_target")
				if err != nil {
					return out, err
				}
			}
		}
		selectedApps, selectedTargets := []uuid.UUID{}, []uuid.UUID{}
		bindingVersions := map[string]int64{}
		bindingTargets := map[string]uuid.UUID{}
		if in.Applications != nil {
			if in.Applications.Mode == Explicit {
				selectedApps = in.Applications.IDs
			} else {
				selectedApps, err = r.sourceGrantIDs(ctx, a, "application")
				if err != nil {
					return out, err
				}
			}
			bindings, e := r.Queries.ListMonitoringBindings(ctx, db.ListMonitoringBindingsParams{EnterpriseID: a.EnterpriseID, Column2: selectedApps})
			if e != nil {
				return out, e
			}
			for _, binding := range bindings {
				bindingVersions[binding.ID.String()] = binding.Version
				bindingTargets[binding.ID.String()] = binding.TargetID
				if binding.TargetType == "monitoring_target" && !slices.Contains(selectedTargets, binding.TargetID) {
					selectedTargets = append(selectedTargets, binding.TargetID)
				}
			}
		}
		scopes := []sourceScope{}
		for _, resource := range out.Authority.Resources {
			scopes = append(scopes, sourceScope{Type: resource.Kind, ID: resource.ID, ClusterID: resource.ParentID, NamespaceUID: resource.NamespaceUID, AllowInfrastructure: in.Applications == nil || resource.ApplicationLinked})
		}
		raw, _ := json.Marshal(scopes)
		signal := in.Signal
		if signal == "catalog" {
			signal = ""
		}
		rows, e := r.Queries.ListObservationSources(ctx, db.ListObservationSourcesParams{EnterpriseID: a.EnterpriseID, ScopeRefs: raw, Signal: signal, SourceType: in.SourceType, OperationalLogs: operational, ApplicationIds: applications, TargetIds: targets, FilterApplications: in.Applications != nil, SelectedApplicationIds: selectedApps, SelectedTargetIds: selectedTargets})
		if e != nil {
			return out, e
		}
		if len(rows) > 10000 {
			return out, ErrInvalid
		}
		for _, row := range rows {
			key := row.ID.String() + ":" + strconv.FormatInt(row.ConfigRevision, 10)
			allowed := boundedSourceKeys(key, row, out.Authority.Resources)
			selected := []string{}
			for _, selector := range allowed {
				if in.Keys == nil || requested[key] || requested[selector] {
					selected = append(selected, selector)
				}
			}
			if len(selected) == 0 {
				continue
			}
			delete(requested, key)
			for _, selector := range selected {
				delete(requested, selector)
			}
			out.Rows = append(out.Rows, row)
			if row.ResourceType != "application" {
				for id, target := range bindingTargets {
					if target == row.ResourceID {
						out.Relations[id] = bindingVersions[id]
						if out.SourceRelations[key] == nil {
							out.SourceRelations[key] = map[string]int64{}
						}
						out.SourceRelations[key][id] = bindingVersions[id]
					}
				}
			}
			out.Keys = append(out.Keys, selected...)
			ref := resourceref.Ref{Type: row.ResourceType, ID: row.ResourceID}
			if !slices.Contains(out.DataResources, ref) {
				out.DataResources = append(out.DataResources, ref)
			}
		}
	}
	if len(requested) > 0 {
		return out, ErrDenied
	}
	out.DataResources, err = resourceref.Normalize(out.DataResources)
	if err != nil {
		return out, ErrInvalid
	}
	sort.Strings(out.Keys)
	material, _ := json.Marshal(struct {
		Authority string
		Data      []resourceref.Ref
		Keys      []string
		Type      string
		Relations map[string]int64
	}{out.Authority.Signature, out.DataResources, out.Keys, in.SourceType, out.Relations})
	hash := sha256.Sum256(material)
	out.Signature = hex.EncodeToString(hash[:])
	return out, nil
}

func (r Resolver) sourceGrantIDs(ctx context.Context, a authorization.Actor, kind string) ([]uuid.UUID, error) {
	if r.Permission == "" {
		if err := authorization.Authorize(ctx, r.Queries, a, RootPermission(kind)); err != nil {
			if errors.Is(err, ErrDenied) {
				return []uuid.UUID{}, nil
			}
			return nil, err
		}
	}
	return authorization.AuthorizedResourceIDs(ctx, r.Queries, a, kind)
}
