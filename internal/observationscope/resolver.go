// Package observationscope freezes authorized telemetry roots and current
// application dependencies before any aggregation, candidate query or cache.
package observationscope

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const Version = "argus.observation_scope/v3"
const AllAuthorized = "all_authorized"
const Explicit = "explicit"

var ErrInvalid = errors.New("invalid observation scope")
var ErrDenied = authorization.ErrAccessDenied

type Ref struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}
type Selection struct {
	Mode  string      `json:"mode"`
	IDs   []uuid.UUID `json:"ids"`
	Types []string    `json:"types"`
	// Applications nil leaves the application dimension unrestricted; explicit
	// empty means no applications and must never expand to every resource.
	Applications *ApplicationSelection `json:"applications,omitempty"`
	Signal       string                `json:"signal"`
}
type ApplicationSelection struct {
	Mode string      `json:"mode"`
	IDs  []uuid.UUID `json:"ids"`
}
type Resource struct {
	Ref
	Name              string            `json:"name"`
	Environment       string            `json:"environment"`
	Labels            map[string]string `json:"labels"`
	Version           int64             `json:"version"`
	ParentID          *uuid.UUID        `json:"parent_resource_id,omitempty"`
	ParentVersion     int64             `json:"parent_resource_version,omitempty"`
	NamespaceUID      string            `json:"namespace_uid,omitempty"`
	ApplicationLinked bool              `json:"application_linked,omitempty"`
}
type Snapshot struct {
	Query                *SourceResult    `json:"-"`
	SchemaVersion        string           `json:"schema_version"`
	EnterpriseID         uuid.UUID        `json:"enterprise_id"`
	SubjectType          string           `json:"subject_type"`
	SubjectID            uuid.UUID        `json:"subject_id"`
	AuthorizationVersion int64            `json:"authorization_version"`
	Selection            Selection        `json:"selection"`
	Resources            []Resource       `json:"resources"`
	Relations            map[string]int64 `json:"relations"`
	Partial              bool             `json:"partial"`
	Signature            string           `json:"signature"`
}

func (s Snapshot) DataRefs() []resourceref.Ref {
	if s.Query != nil {
		return s.Query.DataResources
	}
	return s.Refs()
}
func (s Snapshot) DataIDs() []uuid.UUID { return resourceref.IDs(s.DataRefs()) }
func (s Snapshot) QuerySignature() string {
	if s.Query != nil {
		return s.Query.Signature
	}
	return s.Signature
}

type Resolver struct {
	Queries *db.Queries
	// Permission is the entrypoint capability, independent of object grants.
	// Dashboard access does not require opening the resource management menu.
	// Empty uses each resource page's own read capability.
	Permission string
}

func (s Snapshot) Refs() []resourceref.Ref {
	out := make([]resourceref.Ref, 0, len(s.Resources))
	for _, r := range s.Resources {
		out = append(out, resourceref.Ref{Type: r.Kind, ID: r.ID})
	}
	return out
}

func RootPermission(kind string) string {
	switch kind {
	case "host":
		return "host.read"
	case "kubernetes_cluster":
		return "kubernetes.read"
	case "kubernetes_namespace":
		return "kubernetes.read"
	case "monitoring_target":
		return "monitoring.read"
	case "application":
		return "application.read"
	default:
		return ""
	}
}

func normalizeIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) > 1000 {
		return nil, ErrInvalid
	}
	result := append([]uuid.UUID{}, ids...)
	for _, id := range result {
		if id == uuid.Nil {
			return nil, ErrInvalid
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return slices.Compact(result), nil
}

func normalize(in Selection) (Selection, error) {
	if (in.Mode != AllAuthorized && in.Mode != Explicit) || !slices.Contains([]string{"metrics", "logs", "traces", "catalog"}, in.Signal) {
		return in, ErrInvalid
	}
	var err error
	in.IDs, err = normalizeIDs(in.IDs)
	if err != nil || (in.Mode == AllAuthorized && len(in.IDs) > 0) {
		return in, ErrInvalid
	}
	if len(in.Types) == 0 {
		in.Types = []string{"application", "host", "kubernetes_cluster", "monitoring_target", "kubernetes_namespace"}
	} else {
		in.Types = slices.Clone(in.Types)
	}
	for _, kind := range in.Types {
		if RootPermission(kind) == "" {
			return in, ErrInvalid
		}
	}
	sort.Strings(in.Types)
	in.Types = slices.Compact(in.Types)
	if in.Applications != nil {
		copy := *in.Applications
		in.Applications = &copy
		copy.IDs, err = normalizeIDs(copy.IDs)
		if err != nil || (copy.Mode != Explicit && copy.Mode != AllAuthorized) || (copy.Mode == AllAuthorized && len(copy.IDs) > 0) {
			return in, ErrInvalid
		}
	}
	return in, nil
}

func (r Resolver) Resolve(ctx context.Context, a authorization.Actor, input Selection) (Snapshot, error) {
	return r.resolve(ctx, a, input, true)
}

// Directory uses the same grants and metadata rules before callers paginate.
// It is not an executable query scope; execution still applies the 1000-root budget.
func (r Resolver) Directory(ctx context.Context, a authorization.Actor, input Selection) (Snapshot, error) {
	return r.resolve(ctx, a, input, false)
}

func (r Resolver) resolve(ctx context.Context, a authorization.Actor, input Selection, execution bool) (Snapshot, error) {
	in, err := normalize(input)
	out := Snapshot{SchemaVersion: Version, EnterpriseID: a.EnterpriseID, SubjectType: a.SubjectType, SubjectID: a.SubjectID, AuthorizationVersion: a.AuthorizationVersion, Selection: in, Resources: []Resource{}, Relations: map[string]int64{}}
	if err != nil || r.Queries == nil {
		return out, ErrInvalid
	}
	byRef := map[Ref]Resource{}
	permissions := []string{}
	if r.Permission != "" {
		if err := authorization.Authorize(ctx, r.Queries, a, r.Permission); err != nil {
			return out, err
		}
		permissions = append(permissions, r.Permission)
	}
	types := slices.Clone(in.Types)
	if in.Applications != nil && !slices.Contains(types, "application") {
		types = append(types, "application")
	}
	for _, kind := range types {
		permission := RootPermission(kind)
		if r.Permission == "" {
			if err := authorization.Authorize(ctx, r.Queries, a, permission); err != nil {
				if errors.Is(err, ErrDenied) {
					continue
				}
				return out, err
			}
			permissions = append(permissions, permission)
		}
		needsApplicationCheck := kind == "application" && in.Applications != nil && len(in.Applications.IDs) > 0
		if in.Mode == Explicit && len(in.IDs) == 0 && !needsApplicationCheck || in.Applications != nil && in.Applications.Mode == Explicit && len(in.Applications.IDs) == 0 {
			continue
		}
		ids, err := authorization.AuthorizedResourceIDs(ctx, r.Queries, a, kind)
		if err != nil {
			return out, err
		}
		rows, err := r.Queries.ListObservationScopeResources(ctx, db.ListObservationScopeResourcesParams{EnterpriseID: a.EnterpriseID, Kind: kind, Ids: ids})
		if err != nil {
			return out, err
		}
		for _, row := range rows {
			labels := map[string]string{}
			if json.Unmarshal(row.Labels, &labels) != nil {
				return out, ErrInvalid
			}
			ref := Ref{Kind: row.Kind, ID: row.ID}
			item := Resource{Ref: ref, Name: row.Name, Environment: row.Environment, Labels: labels, Version: row.Version, NamespaceUID: row.NamespaceUid, ParentVersion: row.ParentVersion}
			if row.ParentID != uuid.Nil {
				id := row.ParentID
				item.ParentID = &id
			}
			byRef[ref] = item
		}
	}
	if len(permissions) == 0 {
		return out, ErrDenied
	}
	selected := map[Ref]bool{}
	if in.Applications != nil {
		apps := []uuid.UUID{}
		for ref := range byRef {
			if ref.Kind == "application" && (in.Applications.Mode == AllAuthorized || slices.Contains(in.Applications.IDs, ref.ID)) {
				apps = append(apps, ref.ID)
				selected[ref] = true
			}
		}
		for _, id := range in.Applications.IDs {
			if !slices.Contains(apps, id) {
				return out, ErrDenied
			}
		}
		// Shared resource metrics use current relations. Application logs and
		// traces keep their original application root regardless of later links.
		if (in.Signal == "metrics" || in.Signal == "catalog") && !(in.Mode == Explicit && len(in.IDs) == 0) {
			bindings, err := r.Queries.ListMonitoringBindings(ctx, db.ListMonitoringBindingsParams{EnterpriseID: a.EnterpriseID, Column2: apps})
			if err != nil {
				return out, err
			}
			for _, binding := range bindings {
				if binding.TargetType == "kubernetes_cluster" {
					for nsRef, namespace := range byRef {
						if nsRef.Kind == "kubernetes_namespace" && namespace.ParentID != nil && *namespace.ParentID == binding.TargetID {
							namespace.ApplicationLinked = true
							byRef[nsRef] = namespace
							selected[nsRef] = true
							out.Relations[binding.ID.String()] = binding.Version
						}
					}
				}
				if !slices.Contains(in.Types, binding.TargetType) {
					continue
				}
				ref := Ref{Kind: binding.TargetType, ID: binding.TargetID}
				if _, authorized := byRef[ref]; authorized {
					selected[ref] = true
					out.Relations[binding.ID.String()] = binding.Version
				} else {
					out.Partial = true
				}
			}
		}
	}
	foundIDs := map[uuid.UUID]bool{}
	for ref, item := range byRef {
		if !slices.Contains(in.Types, ref.Kind) {
			continue
		}
		if in.Mode == Explicit && !slices.Contains(in.IDs, ref.ID) {
			continue
		}
		if in.Applications != nil && !selected[ref] {
			if ref.Kind != "kubernetes_namespace" || len(selected) == 0 || in.Signal != "logs" && in.Signal != "catalog" {
				continue
			}
		}
		if foundIDs[ref.ID] {
			return out, ErrInvalid
		} // ambiguous cross-kind identity
		foundIDs[ref.ID] = true
		out.Resources = append(out.Resources, item)
	}
	if in.Mode == Explicit {
		for _, id := range in.IDs {
			// An authorized resource excluded by the application filter is an
			// empty match; an ungranted explicit ID is an authorization failure.
			if !slices.ContainsFunc(mapValues(byRef), func(v Resource) bool { return v.ID == id && slices.Contains(in.Types, v.Kind) }) {
				return out, ErrDenied
			}
		}
	}
	if execution && len(out.Resources) > 1000 {
		return out, ErrInvalid
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		if out.Resources[i].Kind != out.Resources[j].Kind {
			return out.Resources[i].Kind < out.Resources[j].Kind
		}
		return out.Resources[i].ID.String() < out.Resources[j].ID.String()
	})
	for _, permission := range permissions {
		if err = authorization.Authorize(ctx, r.Queries, a, permission); err != nil {
			return Snapshot{}, err
		}
	}
	raw, _ := json.Marshal(out)
	digest := sha256.Sum256(raw)
	out.Signature = hex.EncodeToString(digest[:])
	return out, nil
}

func mapValues(values map[Ref]Resource) []Resource {
	result := make([]Resource, 0, len(values))
	for _, item := range values {
		result = append(result, item)
	}
	return result
}

func (s Snapshot) IDs() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(s.Resources))
	for _, item := range s.Resources {
		ids = append(ids, item.ID)
	}
	return ids
}
