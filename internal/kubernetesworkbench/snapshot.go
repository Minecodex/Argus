package kubernetesworkbench

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"slices"
	"sort"
	"time"
)

type SnapshotInput struct {
	Types      []string  `json:"types,omitempty"`
	Namespaces Selection `json:"namespaces"`
	MaxItems   int       `json:"max_items,omitempty"`
}
type ResourceReference struct {
	Type         string     `json:"type"`
	Name         string     `json:"name"`
	UID          string     `json:"uid"`
	Namespace    string     `json:"namespace"`
	NamespaceID  *uuid.UUID `json:"namespace_id,omitempty"`
	NamespaceUID string     `json:"namespace_uid,omitempty"`
}
type SnapshotResource struct {
	ResourceReference
	Summary   kubernetesinventory.Summary              `json:"summary"`
	Actions   []kubernetesinventory.ActionAvailability `json:"actions"`
	CreatedAt time.Time                                `json:"created_at"`
}
type ResourceRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}
type Snapshot struct {
	FocusCandidates []ResourceReference       `json:"focus_candidates,omitempty"`
	FocusUID        string                    `json:"focus_uid,omitempty"`
	ClusterID       uuid.UUID                 `json:"cluster_id"`
	ClusterWide     bool                      `json:"cluster_wide"`
	Resources       []SnapshotResource        `json:"resources"`
	Relations       []ResourceRelation        `json:"relations"`
	Counts          map[string]map[string]int `json:"counts"`
	Complete        bool                      `json:"complete"`
	Errors          map[string]string         `json:"errors"`
	ObservedAt      time.Time                 `json:"observed_at"`
	ScopeSignature  string                    `json:"scope_signature"`
}

var snapshotTypes = []string{"node", "pod", "deployment", "replicaset", "statefulset", "daemonset", "hpa", "pdb", "job", "cronjob", "pvc", "pv", "storageclass", "service", "ingress", "ingressclass", "networkpolicy", "configmap", "secret", "endpointslice", "serviceaccount", "event"}

func DefaultSnapshotTypes() []string { return slices.Clone(snapshotTypes) }

func (s Service) Snapshot(ctx context.Context, a Actor, clusterID uuid.UUID, in SnapshotInput) (Snapshot, error) {
	out := Snapshot{ClusterID: clusterID, Resources: []SnapshotResource{}, Relations: []ResourceRelation{}, Counts: map[string]map[string]int{}, Errors: map[string]string{}, Complete: true, ObservedAt: time.Now().UTC()}
	_, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return out, err
	}
	out.ClusterWide = scope.ClusterWide
	out.ScopeSignature = scopeSignature(scope, scope.Namespaces)
	if in.MaxItems == 0 {
		in.MaxItems = 1000
	}
	if in.MaxItems < 1 || in.MaxItems > 2000 || len(in.Types) > 32 {
		return out, kubernetesinventory.ErrInvalid
	}
	if len(in.Types) == 0 {
		in.Types = slices.Clone(snapshotTypes)
	}
	seen := map[string]bool{}
	for _, key := range in.Types {
		if seen[key] {
			return out, kubernetesinventory.ErrInvalid
		}
		seen[key] = true
		if _, ok := kubernetesinventory.Lookup(key); !ok {
			return out, kubernetesinventory.ErrUnsupported
		}
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	objects := []kubernetesinventory.Object{}
	refs := map[string]uuid.UUID{}
	for _, key := range in.Types {
		definition, _ := kubernetesinventory.Lookup(key)
		if !definition.Namespaced && !scope.ClusterWide {
			continue
		}
		out.Counts[key] = map[string]int{"total": 0}
		cursor := ""
		selection := in.Namespaces
		if !definition.Namespaced {
			selection = Selection{}
		}
		for {
			remaining := in.MaxItems - len(objects)
			if remaining <= 0 {
				out.Complete = false
				out.Errors[key] = "KUBERNETES_RESULT_BUDGET_EXCEEDED"
				break
			}
			page, e := s.List(ctx, a, clusterID, ListInput{Type: key, Namespaces: selection, Limit: min(remaining, 200), Cursor: cursor})
			if e != nil {
				if errors.Is(e, authorization.ErrAccessDenied) {
					return Snapshot{}, e
				}
				out.Complete = false
				out.Errors[key] = kubernetesinventory.ErrorCode(e)
				break
			}
			objects = append(objects, page.Items...)
			for uid, id := range page.NamespaceReferences {
				refs[uid] = id
			}
			if page.NextCursor == "" {
				if !page.Complete {
					out.Complete = false
					out.Errors[key] = "KUBERNETES_RESULT_BUDGET_EXCEEDED"
				}
				break
			}
			if page.NextCursor == cursor {
				out.Complete = false
				out.Errors[key] = "KUBERNETES_READER_VERSION_UNSUPPORTED"
				break
			}
			cursor = page.NextCursor
		}
	}
	for _, object := range objects {
		summary := kubernetesinventory.Summarize(object)
		ref := ResourceReference{Type: object.Definition.Key, Name: object.Name, UID: object.UID, Namespace: object.Namespace, NamespaceUID: object.NamespaceUID}
		if id, ok := refs[object.NamespaceUID]; ok {
			ref.NamespaceID = &id
		}
		out.Resources = append(out.Resources, SnapshotResource{ResourceReference: ref, Summary: summary, Actions: object.Actions, CreatedAt: object.CreatedAt})
		if out.Counts[ref.Type] == nil {
			out.Counts[ref.Type] = map[string]int{}
		}
		out.Counts[ref.Type][summary.State]++
		out.Counts[ref.Type]["total"]++
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		a, b := out.Resources[i], out.Resources[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	out.Relations = resourceRelations(objects)
	_, latest, e := s.scope(parent, a, clusterID)
	if e != nil {
		return Snapshot{}, e
	}
	if scopeSignature(latest, latest.Namespaces) != out.ScopeSignature {
		return Snapshot{}, authorization.ErrAccessDenied
	}
	return out, nil
}
