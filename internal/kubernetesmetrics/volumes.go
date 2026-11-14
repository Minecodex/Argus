package kubernetesmetrics

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"slices"
	"strings"
	"time"
)

func (s Service) readPVCs(ctx context.Context, a kubernetesworkbench.Actor, state preparedState, out Result) (Result, error) {
	trends := s.volumeTrends(ctx, a, state, out.From, out.To)
	for _, r := range state.Rows {
		v, ok := trends[r.UID]
		if !ok {
			v = Trend{Kind: "volume", Unit: "bytes", State: "no_data", Basis: "usage", Points: []Point{}}
		}
		if r.Changed {
			v.State = "identity_changed"
		}
		out.Items = append(out.Items, Entry{UID: r.UID, Trends: []Trend{v}})
	}
	if e := s.validate(ctx, a, state); e != nil {
		return Result{}, e
	}
	return out, nil
}
func (s Service) appendVolumes(ctx context.Context, a kubernetesworkbench.Actor, state preparedState, out *Result) {
	values := s.volumeTrends(ctx, a, state, out.From, out.To)
	for i := range out.Items {
		if v, ok := values[out.Items[i].UID]; ok {
			out.Items[i].Trends = append(out.Items[i].Trends, v)
		}
	}
}
func (s Service) volumeTrends(ctx context.Context, a kubernetesworkbench.Actor, state preparedState, from, to time.Time) map[string]Trend {
	out := map[string]Trend{}
	nsIDs := []uuid.UUID{}
	for _, r := range state.Rows {
		if r.NamespaceID != uuid.Nil && !slices.Contains(nsIDs, r.NamespaceID) {
			nsIDs = append(nsIDs, r.NamespaceID)
		}
	}
	if len(nsIDs) == 0 {
		return out
	}
	selection := kubernetesworkbench.Selection{Mode: "explicit", IDs: nsIDs}
	pods := state.Pods
	if state.Type == "pvc" {
		objects, refs, done, e := s.scan(ctx, a, state.Cluster, "pod", selection, 500)
		if e != nil || !done {
			for _, r := range state.Rows {
				out[r.UID] = Trend{Kind: "volume", Unit: "bytes", State: "partial", Reason: "INCOMPLETE", Basis: "usage", Points: []Point{}}
			}
			return out
		}
		for _, o := range objects {
			pods = append(pods, podFrom(o, refs))
		}
	}
	parents := map[string][]string{}
	hasVolumes := false
	for _, p := range pods {
		for _, name := range p.Volumes {
			hasVolumes = true
			key := p.Namespace + "/" + name
			parents[key] = append(parents[key], p.Parents...)
		}
	}
	if state.Type != "pvc" && !hasVolumes {
		return out
	}
	pvcs, _, complete, e := s.scan(ctx, a, state.Cluster, "pvc", selection, 500)
	if e != nil || !complete {
		for _, r := range state.Rows {
			out[r.UID] = Trend{Kind: "volume", Unit: "bytes", State: "partial", Reason: "INCOMPLETE", Basis: "usage", Points: []Point{}}
		}
		return out
	}
	byName := map[string]kubernetesinventory.Object{}
	names, nss, podIDs := []string{}, []string{}, []string{}
	selected := map[string]bool{}
	for _, r := range state.Rows {
		selected[r.UID] = true
	}
	for _, o := range pvcs {
		key := o.Namespace + "/" + o.Name
		if state.Type == "pvc" {
			if !selected[o.UID] {
				continue
			}
			parents[key] = []string{o.UID}
		} else if len(parents[key]) == 0 {
			continue
		}
		byName[key] = o
		names = append(names, o.Name)
		nss = append(nss, o.Namespace)
	}
	for _, p := range pods {
		for _, name := range p.Volumes {
			o, ok := byName[p.Namespace+"/"+name]
			if ok && !p.Created.Before(o.CreatedAt) {
				podIDs = append(podIDs, p.UID)
			}
		}
	}
	data := map[string]series{}
	code := ""
	if len(names) > 0 && len(podIDs) > 0 {
		selector := strings.TrimSuffix(match("resource_k8s_persistentvolumeclaim_name", names), "}") + "," + strings.TrimPrefix(match("resource_k8s_namespace_name", nss), "{")
		selector = strings.TrimSuffix(selector, "}") + "," + strings.TrimPrefix(match("resource_k8s_pod_uid", podIDs), "{")
		group := "resource_k8s_namespace_name,resource_k8s_persistentvolumeclaim_name"
		capMetric := "k8s_volume_capacity" + selector
		available := "k8s_volume_available" + selector
		expression := component("max by ("+group+") ("+capMetric+" - "+available+")", "used") + " or " + component("max by ("+group+") ("+capMetric+")", "capacity") + " or " + component("max by ("+group+") (timestamp("+capMetric+"))", "sample_time")
		data, e = s.query(ctx, a, state, "kubeletstats", expression, from, to)
		if e != nil {
			code = reason(e)
		}
	}
	for _, r := range state.Rows {
		keys := []string{}
		for key, ids := range parents {
			if slices.Contains(ids, r.UID) {
				if _, ok := byName[key]; ok {
					keys = append(keys, key)
				}
			}
		}
		if len(keys) == 0 && state.Type != "pvc" {
			continue
		}
		v := Trend{Kind: "volume", Unit: "bytes", State: "no_data", Basis: "capacity", Points: []Point{}}
		seen, fresh := false, false
		for at := from; !at.After(to); at = at.Add(30 * time.Second) {
			point := Point{Time: at}
			used, cap := 0., 0.
			count := 0
			for _, key := range keys {
				o := byName[key]
				if at.Before(o.CreatedAt) {
					continue
				}
				n, nOK := sample(data, key, "used", at.Unix())
				c, cOK := sample(data, key, "capacity", at.Unix())
				st, stOK := sample(data, key, "sample_time", at.Unix())
				if nOK && cOK && stOK && float64(at.Unix())-st <= 90 && st >= float64(o.CreatedAt.Unix()) && c > 0 {
					used += n
					cap += c
					count++
				}
			}
			if len(keys) > 0 && count == len(keys) {
				point.Value = number(used)
				point.Capacity = number(cap)
				seen = true
				if at.Unix() >= to.Unix()-60 {
					fresh = true
				}
			}
			v.Points = append(v.Points, point)
		}
		if seen {
			v.State = "ready"
			if !fresh {
				v.State = "stale"
			}
		}
		if code != "" {
			v.State = stateFor(code)
			v.Reason = code
		}
		out[r.UID] = v
	}
	return out
}
