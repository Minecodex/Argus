package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type ListInput struct {
	Type       string    `json:"type"`
	Namespaces Selection `json:"namespaces"`
	Name       string    `json:"name"`
	Labels     string    `json:"labels"`
	Limit      int       `json:"limit"`
	Cursor     string    `json:"cursor"`
}
type Page struct {
	WatchToken          string                       `json:"watch_token,omitempty"`
	Items               []kubernetesinventory.Object `json:"items"`
	NextCursor          string                       `json:"next_cursor,omitempty"`
	Complete            bool                         `json:"complete"`
	ObservedAt          time.Time                    `json:"observed_at"`
	ScopeSignature      string                       `json:"scope_signature"`
	NamespaceReferences map[string]uuid.UUID         `json:"namespace_references"`
}
type position struct {
	Namespace int    `json:"namespace"`
	Continue  string `json:"continue"`
}

func (s Service) List(ctx context.Context, a Actor, clusterID uuid.UUID, in ListInput) (Page, error) {
	out := Page{Items: []kubernetesinventory.Object{}, Complete: true, ObservedAt: time.Now().UTC(), NamespaceReferences: map[string]uuid.UUID{}}
	if in.Limit == 0 {
		in.Limit = 50
	}
	if in.Limit < 1 || in.Limit > 200 || len(in.Cursor) > 32768 {
		return out, kubernetesinventory.ErrInvalid
	}
	cluster, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return out, err
	}
	definition, err := s.definition(ctx, cluster, in.Type)
	if err != nil {
		return out, err
	}
	var namespaces []Namespace
	all := false
	if definition.Namespaced {
		namespaces, all, err = s.selectedNamespaces(ctx, a, scope, in.Namespaces)
		if err != nil {
			return out, err
		}
	} else if !scope.ClusterWide {
		if in.Type != "namespace" {
			return out, authorization.ErrAccessDenied
		}
		return s.namespaceDirectory(ctx, a, cluster, scope, in)
	} else if len(in.Namespaces.IDs) > 0 {
		return out, kubernetesinventory.ErrInvalid
	}
	if !definition.Namespaced || all {
		namespaces = []Namespace{{}}
	}
	out.ScopeSignature = scopeSignature(scope, namespaces)
	filter := in
	filter.Cursor = ""
	binding := pagination.Binding{Audience: "kubernetes_inventory/v2", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter(struct {
		Input ListInput
		Scope string
	}{filter, out.ScopeSignature}), Sort: "kubernetes-server-snapshot"}
	signer := pagination.Signer{Key: s.CursorKey}
	next := position{}
	if in.Cursor != "" {
		decoded, e := signer.Decode(in.Cursor, binding)
		if e != nil {
			return out, e
		}
		if json.Unmarshal([]byte(decoded.ID), &next) != nil || next.Namespace < 0 || next.Namespace >= len(namespaces) {
			return out, kubernetesinventory.ErrInvalid
		}
	}
	queries := 0
	watchPositions := []watchPosition{}
	for next.Namespace < len(namespaces) && len(out.Items) < in.Limit && queries < 10 {
		namespace := namespaces[next.Namespace]
		query := kubernetesinventory.Query{Type: in.Type, Namespace: namespace.Name, NamespaceUID: namespace.UID, AllNamespaces: all, Name: in.Name, Labels: in.Labels, Limit: int64(in.Limit - len(out.Items)), Continue: next.Continue}
		page, e := s.Backend.ListInventory(ctx, cluster, query)
		if e != nil {
			return Page{}, e
		}
		if page.SchemaVersion != kubernetesinventory.SchemaVersion || page.ResourceVersion == "" || page.ObservedAt.IsZero() || len(page.Items) > int(query.Limit) || page.Continue != "" && page.Continue == query.Continue {
			return Page{}, kubernetesinventory.ErrProtocol
		}
		for _, object := range page.Items {
			if object.Definition != definition || object.UID == "" || namespace.ID != uuid.Nil && (object.Namespace != namespace.Name || object.NamespaceUID != namespace.UID) || !definition.Namespaced && object.Namespace != "" {
				return Page{}, kubernetesinventory.ErrIdentityChanged
			}
		}
		queries++
		watchPositions = append(watchPositions, watchPosition{Namespace: namespace, AllNamespaces: all, ResourceVersion: page.ResourceVersion})
		out.Items = append(out.Items, page.Items...)
		out.ObservedAt = page.ObservedAt
		if namespace.ID != uuid.Nil {
			out.NamespaceReferences[namespace.UID] = namespace.ID
		}
		if in.Type == "namespace" {
			refs, e := s.observeNamespaces(ctx, a.EnterpriseID, clusterID, page.Items)
			if e != nil {
				return Page{}, e
			}
			for uid, id := range refs {
				out.NamespaceReferences[uid] = id
			}
		}
		if all && definition.Namespaced {
			seen := map[string]bool{}
			observed := []kubernetesinventory.Object{}
			namespaceDefinition, _ := kubernetesinventory.Lookup("namespace")
			for _, object := range page.Items {
				if object.NamespaceUID == "" {
					return Page{}, kubernetesinventory.ErrIdentityChanged
				}
				if !seen[object.NamespaceUID] {
					seen[object.NamespaceUID] = true
					observed = append(observed, kubernetesinventory.Object{Definition: namespaceDefinition, UID: object.NamespaceUID, Name: object.Namespace})
				}
			}
			refs, e := s.observeNamespaces(ctx, a.EnterpriseID, clusterID, observed)
			if e != nil {
				return Page{}, e
			}
			for uid, id := range refs {
				out.NamespaceReferences[uid] = id
			}
		}
		if page.Continue != "" {
			next.Continue = page.Continue
			break
		}
		next.Namespace++
		next.Continue = ""
	}
	if next.Namespace < len(namespaces) {
		raw, _ := json.Marshal(next)
		out.NextCursor, err = signer.Encode(binding, pagination.Position{Time: out.ObservedAt, ID: string(raw)})
		if err != nil {
			return Page{}, err
		}
		out.Complete = false
	}
	_, current, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return Page{}, err
	}
	if scopeSignature(current, namespaces) != out.ScopeSignature {
		return Page{}, authorization.ErrAccessDenied
	}
	if len(watchPositions) > 0 && len(watchPositions) <= 4 {
		out.WatchToken, err = s.issueWatch(a, clusterID, watchState{Input: filter, ScopeSignature: out.ScopeSignature, Positions: watchPositions})
		if err != nil {
			return Page{}, err
		}
	}
	return out, nil
}

func (s Service) observeNamespaces(ctx context.Context, enterprise, cluster uuid.UUID, objects []kubernetesinventory.Object) (map[string]uuid.UUID, error) {
	result := map[string]uuid.UUID{}
	err := s.Store.InTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockNamespaceInventoryCluster(ctx, db.LockNamespaceInventoryClusterParams{EnterpriseID: enterprise, ID: cluster}); err != nil {
			return err
		}
		for _, object := range objects {
			if object.Definition.Key != "namespace" || object.Namespace != "" || object.UID == "" {
				return kubernetesinventory.ErrInvalid
			}
			if err := q.RetireReplacedKubernetesNamespace(ctx, db.RetireReplacedKubernetesNamespaceParams{EnterpriseID: enterprise, ClusterID: cluster, Name: object.Name, NamespaceUid: object.UID}); err != nil {
				return err
			}
			row, err := q.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.NewSHA1(cluster, []byte(object.UID)), EnterpriseID: enterprise, ClusterID: cluster, NamespaceUid: object.UID, Name: object.Name, ResourceVersion: object.ResourceVersion})
			if err != nil {
				return err
			}
			result[object.UID] = row.ID
		}
		return nil
	})
	return result, err
}

func (s Service) namespaceDirectory(ctx context.Context, a Actor, cluster db.KubernetesCluster, scope Scope, in ListInput) (Page, error) {
	// Namespace-only subjects receive only their UID-bound directory, never a
	// broad cluster namespace query followed by post-filtering.
	out := Page{Items: []kubernetesinventory.Object{}, Complete: true, ObservedAt: time.Now().UTC(), ScopeSignature: scopeSignature(scope, scope.Namespaces), NamespaceReferences: map[string]uuid.UUID{}}
	binding := pagination.Binding{Audience: "kubernetes_namespace_directory/v2", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter(struct {
		Scope, Name string
		Limit       int
	}{out.ScopeSignature, in.Name, in.Limit}), Sort: "namespace-id"}
	signer := pagination.Signer{Key: s.CursorKey}
	after := ""
	if in.Cursor != "" {
		position, err := signer.Decode(in.Cursor, binding)
		if err != nil {
			return out, err
		}
		after = position.ID
	}
	for _, namespace := range scope.Namespaces {
		if namespace.ID.String() <= after || in.Name != "" && namespace.Name != in.Name {
			continue
		}
		if len(out.Items) == in.Limit {
			out.Complete = false
			break
		}
		object, err := s.Backend.GetInventory(ctx, cluster, kubernetesinventory.Query{Type: "namespace", Name: namespace.Name, UID: namespace.UID})
		if err != nil {
			return Page{}, err
		}
		if object.Definition.Key != "namespace" || object.UID != namespace.UID || object.Name != namespace.Name || object.Namespace != "" {
			return Page{}, kubernetesinventory.ErrIdentityChanged
		}
		out.Items = append(out.Items, object)
		out.NamespaceReferences[namespace.UID] = namespace.ID
		after = namespace.ID.String()
	}
	if !out.Complete {
		var err error
		out.NextCursor, err = signer.Encode(binding, pagination.Position{Time: out.ObservedAt, ID: after})
		if err != nil {
			return Page{}, err
		}
	}
	_, current, err := s.scope(ctx, a, cluster.ID)
	if err != nil {
		return Page{}, err
	}
	if scopeSignature(current, current.Namespaces) != out.ScopeSignature {
		return Page{}, authorization.ErrAccessDenied
	}
	return out, nil
}
