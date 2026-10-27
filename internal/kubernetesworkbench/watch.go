package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"golang.org/x/sync/errgroup"
)

type watchBackend interface {
	WatchInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.WatchBatch, error)
}
type watchPosition struct {
	Namespace       Namespace `json:"namespace"`
	AllNamespaces   bool      `json:"all_namespaces"`
	ResourceVersion string    `json:"resource_version"`
}
type watchState struct {
	Input          ListInput       `json:"input"`
	ScopeSignature string          `json:"scope_signature"`
	Positions      []watchPosition `json:"positions"`
}
type WatchResult struct {
	More           bool                        `json:"more"`
	Events         []kubernetesinventory.Event `json:"events"`
	WatchToken     string                      `json:"watch_token"`
	ScopeSignature string                      `json:"scope_signature"`
	ObservedAt     time.Time                   `json:"observed_at"`
}

func watchBinding(a Actor, cluster uuid.UUID) pagination.Binding {
	return pagination.Binding{Audience: "kubernetes_current_page_watch/v1", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: cluster.String(), Sort: "kubernetes-resource-version"}
}
func (s Service) issueWatch(a Actor, cluster uuid.UUID, state watchState) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	value, err := (pagination.Signer{Key: s.CursorKey}).Encode(watchBinding(a, cluster), pagination.Position{Time: time.Now().UTC(), ID: string(raw)})
	if len(value) > 32768 {
		return "", nil
	}
	return value, err
}
func (s Service) Watch(ctx context.Context, a Actor, clusterID uuid.UUID, token string) (WatchResult, error) {
	out := WatchResult{Events: []kubernetesinventory.Event{}, ObservedAt: time.Now().UTC()}
	if token == "" || len(token) > 32768 {
		return out, kubernetesinventory.ErrInvalid
	}
	backend, ok := s.Backend.(watchBackend)
	if !ok {
		return out, kubernetesinventory.ErrProtocol
	}
	decoded, err := (pagination.Signer{Key: s.CursorKey}).Decode(token, watchBinding(a, clusterID))
	if err != nil {
		return out, err
	}
	var state watchState
	if json.Unmarshal([]byte(decoded.ID), &state) != nil || len(state.Positions) == 0 || len(state.Positions) > 4 {
		return out, kubernetesinventory.ErrInvalid
	}
	cluster, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return out, err
	}
	definition, err := s.definition(ctx, cluster, state.Input.Type)
	if err != nil {
		return out, err
	}
	var namespaces []Namespace
	all := false
	if definition.Namespaced {
		namespaces, all, err = s.selectedNamespaces(ctx, a, scope, state.Input.Namespaces)
		if err != nil {
			return out, err
		}
	} else if !scope.ClusterWide {
		return out, authorization.ErrAccessDenied
	}
	if all || !definition.Namespaced {
		namespaces = []Namespace{{}}
	}
	if scopeSignature(scope, namespaces) != state.ScopeSignature {
		return out, authorization.ErrAccessDenied
	}
	for _, position := range state.Positions {
		found := false
		for _, namespace := range namespaces {
			if namespace == position.Namespace {
				found = true
			}
		}
		if !found || position.AllNamespaces != all || position.ResourceVersion == "" {
			return out, authorization.ErrAccessDenied
		}
	}
	results := make([]kubernetesinventory.WatchBatch, len(state.Positions))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index, position := range state.Positions {
		group.Go(func() error {
			batch, err := backend.WatchInventory(groupContext, cluster, kubernetesinventory.Query{Type: state.Input.Type, Name: state.Input.Name, Labels: state.Input.Labels, Namespace: position.Namespace.Name, NamespaceUID: position.Namespace.UID, AllNamespaces: position.AllNamespaces, ResourceVersion: position.ResourceVersion, Limit: 50})
			if err != nil {
				return err
			}
			if batch.SchemaVersion != kubernetesinventory.SchemaVersion || batch.ResourceVersion == "" || batch.ObservedAt.IsZero() || len(batch.Events) > 50 {
				return kubernetesinventory.ErrProtocol
			}
			for _, event := range batch.Events {
				if !slices.Contains([]string{"ADDED", "MODIFIED", "DELETED"}, event.Type) || event.Object.Definition != definition || event.Object.UID == "" || position.Namespace.ID != uuid.Nil && (event.Object.Namespace != position.Namespace.Name || event.Object.NamespaceUID != position.Namespace.UID) {
					return kubernetesinventory.ErrIdentityChanged
				}
			}
			results[index] = batch
			return nil
		})
	}
	if err = group.Wait(); err != nil {
		return out, err
	}
	_, current, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return out, err
	}
	if scopeSignature(current, namespaces) != state.ScopeSignature {
		return out, authorization.ErrAccessDenied
	}
	for index, batch := range results {
		out.More = out.More || batch.More
		out.Events = append(out.Events, batch.Events...)
		state.Positions[index].ResourceVersion = batch.ResourceVersion
		if batch.ObservedAt.After(out.ObservedAt) {
			out.ObservedAt = batch.ObservedAt
		}
	}
	raw, _ := json.Marshal(out.Events)
	if len(raw) > 1<<20 {
		return WatchResult{}, kubernetesinventory.ErrBudget
	}
	out.WatchToken, err = s.issueWatch(a, clusterID, state)
	out.ScopeSignature = state.ScopeSignature
	return out, err
}
