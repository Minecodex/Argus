package kubernetesmetrics

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/pagination"
	"slices"
	"sort"
	"time"
)

func binding(a kubernetesworkbench.Actor, cluster uuid.UUID) pagination.Binding {
	return pagination.Binding{Audience: "kubernetes_list_metric_context/v1", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: cluster.String(), Sort: "inventory-metric-snapshot"}
}
func (s Service) Prepare(ctx context.Context, a kubernetesworkbench.Actor, cluster uuid.UUID, in PrepareInput) (Prepared, error) {
	var out Prepared
	if s.Inventory == nil || !Supported(in.List.Type) || len(in.UIDs) < 1 || len(in.UIDs) > 50 || in.List.Limit > 50 {
		return out, kubernetesinventory.ErrInvalid
	}
	requested := map[string]bool{}
	for _, id := range in.UIDs {
		if id == "" || len(id) > 128 || requested[id] {
			return out, kubernetesinventory.ErrInvalid
		}
		requested[id] = true
	}
	if in.List.Limit == 0 {
		in.List.Limit = 30
	}
	scope, err := s.Inventory.AccessScope(ctx, a, cluster)
	if err != nil {
		return out, err
	}
	page, err := s.Inventory.List(ctx, a, cluster, in.List)
	if err != nil {
		return out, err
	}
	state := preparedState{Cluster: cluster, ClusterWide: scope.ClusterWide, Scope: pagination.HashFilter(scope), Type: in.List.Type, Rows: []row{}, Pods: []pod{}, Complete: true, Observed: s.now()}
	objects := map[string]kubernetesinventory.Object{}
	nsIDs := []uuid.UUID{}
	for _, o := range page.Items {
		if !requested[o.UID] {
			continue
		}
		objects[o.UID] = o
		r := rowFrom(o, page.NamespaceReferences)
		state.Rows = append(state.Rows, r)
		if r.NamespaceID != uuid.Nil && !slices.Contains(nsIDs, r.NamespaceID) {
			nsIDs = append(nsIDs, r.NamespaceID)
		}
	}
	for _, id := range in.UIDs {
		if _, ok := objects[id]; !ok {
			state.Rows = append(state.Rows, row{UID: id, Changed: true})
			state.Complete = false
		}
	}
	if in.List.Type == "pod" {
		for _, o := range objects {
			p := podFrom(o, page.NamespaceReferences)
			p.Parents = []string{o.UID}
			state.Pods = append(state.Pods, p)
		}
	}
	if in.List.Type != "pod" && in.List.Type != "pvc" {
		selection := kubernetesworkbench.Selection{Mode: "explicit", IDs: nsIDs}
		if in.List.Type == "node" {
			selection = kubernetesworkbench.Selection{Mode: "all_authorized"}
		}
		pods, refs, complete, e := s.scan(ctx, a, cluster, "pod", selection, 500)
		state.Complete = state.Complete && complete
		if e != nil {
			state.PodError = kubernetesinventory.ErrorCode(e)
			state.Complete = false
		}
		owners := map[string]string{}
		if in.List.Type == "deployment" {
			rs, _, done, e := s.scan(ctx, a, cluster, "replicaset", selection, 500)
			state.Complete = state.Complete && done
			if e != nil {
				state.PodError = kubernetesinventory.ErrorCode(e)
				state.Complete = false
			}
			for _, o := range rs {
				for _, owner := range o.Owners {
					if owner.Controller != nil && *owner.Controller {
						owners[o.NamespaceUID+"/"+o.UID] = string(owner.UID)
					}
				}
			}
		}
		for _, o := range pods {
			p := podFrom(o, refs)
			if p.Terminal {
				continue
			}
			for _, r := range state.Rows {
				if r.Changed {
					continue
				}
				match := false
				switch in.List.Type {
				case "namespace":
					match = o.NamespaceUID == r.UID
				case "node":
					match = podNode(o) == r.Name
				default:
					if o.NamespaceUID != r.NamespaceUID {
						continue
					}
					for _, owner := range o.Owners {
						if owner.Controller != nil && *owner.Controller {
							match = string(owner.UID) == r.UID || owners[o.NamespaceUID+"/"+string(owner.UID)] == r.UID
						}
					}
				}
				if match {
					p.Parents = append(p.Parents, r.UID)
				}
			}
			if len(p.Parents) > 0 {
				state.Pods = append(state.Pods, p)
			}
		}
	}
	for i := range state.Rows {
		if in.List.Type != "pvc" {
			state.Rows[i].Budgets = aggregateBudgets(state.Pods, state.Rows[i].UID)
		}
	}
	sort.Slice(state.Rows, func(i, j int) bool { return state.Rows[i].UID < state.Rows[j].UID })
	sort.Slice(state.Pods, func(i, j int) bool { return state.Pods[i].UID < state.Pods[j].UID })
	latest, err := s.Inventory.AccessScope(ctx, a, cluster)
	if err != nil {
		return out, err
	}
	if pagination.HashFilter(latest) != state.Scope {
		return out, authorization.ErrAccessDenied
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return out, err
	}
	token, err := (pagination.Signer{Key: s.Key, Now: s.Now}).Encode(binding(a, cluster), pagination.Position{Time: state.Observed, ID: string(raw)})
	if err != nil {
		return out, err
	}
	if len(token) > 384<<10 {
		return out, kubernetesinventory.ErrBudget
	}
	return Prepared{Token: token, ObservedAt: state.Observed, ExpiresAt: state.Observed.Add(2 * time.Minute), Complete: state.Complete}, nil
}
func (s Service) scan(ctx context.Context, a kubernetesworkbench.Actor, cluster uuid.UUID, kind string, selection kubernetesworkbench.Selection, max int) ([]kubernetesinventory.Object, map[string]uuid.UUID, bool, error) {
	rows := []kubernetesinventory.Object{}
	refs := map[string]uuid.UUID{}
	cursor := ""
	for reads := 0; reads < 5; reads++ {
		p, e := s.Inventory.List(ctx, a, cluster, kubernetesworkbench.ListInput{Type: kind, Namespaces: selection, Limit: min(200, max-len(rows)), Cursor: cursor})
		if e != nil {
			return rows, refs, false, e
		}
		rows = append(rows, p.Items...)
		for k, v := range p.NamespaceReferences {
			refs[k] = v
		}
		if p.Complete {
			return rows, refs, true, nil
		}
		if len(rows) >= max {
			return rows, refs, false, nil
		}
		cursor = p.NextCursor
		if cursor == "" {
			return rows, refs, false, kubernetesinventory.ErrProtocol
		}
	}
	return rows, refs, false, nil
}
func (s Service) decode(a kubernetesworkbench.Actor, cluster uuid.UUID, token string) (preparedState, error) {
	var state preparedState
	if token == "" || len(token) > 384<<10 {
		return state, kubernetesinventory.ErrInvalid
	}
	p, e := (pagination.Signer{Key: s.Key, Now: s.Now}).Decode(token, binding(a, cluster))
	if e != nil {
		return state, e
	}
	if json.Unmarshal([]byte(p.ID), &state) != nil || state.Cluster != cluster || !Supported(state.Type) || len(state.Rows) > 50 || len(state.Pods) > 500 {
		return state, kubernetesinventory.ErrInvalid
	}
	if s.now().After(state.Observed.Add(2*time.Minute)) || state.Observed.After(s.now().Add(time.Second)) {
		return state, kubernetesinventory.ErrExpired
	}
	return state, nil
}
