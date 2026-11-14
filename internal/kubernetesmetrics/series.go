package kubernetesmetrics

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"math"
	"slices"
	"strconv"
	"time"
)

type series map[int64]float64

func parseSeries(data any) (map[string]series, error) {
	raw, e := json.Marshal(data)
	if e != nil {
		return nil, e
	}
	var values []struct {
		Metric map[string]string   `json:"metric"`
		Values [][]json.RawMessage `json:"values"`
	}
	if json.Unmarshal(raw, &values) != nil {
		return nil, kubernetesinventory.ErrProtocol
	}
	out := map[string]series{}
	for _, v := range values {
		id := v.Metric["resource_k8s_pod_uid"]
		if id != "" && v.Metric["resource_k8s_namespace_name"] != "" && v.Metric["resource_k8s_pod_name"] != "" {
			id = v.Metric["resource_k8s_namespace_name"] + "/" + v.Metric["resource_k8s_pod_name"] + "/" + id
		}
		if id == "" {
			id = v.Metric["resource_k8s_node_name"]
		}
		if id == "" {
			id = v.Metric["resource_k8s_namespace_name"] + "/" + v.Metric["resource_k8s_persistentvolumeclaim_name"]
		}
		key := id + "/" + v.Metric["argus_trend_component"]
		if out[key] == nil {
			out[key] = series{}
		}
		for _, pair := range v.Values {
			if len(pair) != 2 {
				return nil, kubernetesinventory.ErrProtocol
			}
			var stamp float64
			var text string
			if json.Unmarshal(pair[0], &stamp) != nil || json.Unmarshal(pair[1], &text) != nil {
				return nil, kubernetesinventory.ErrProtocol
			}
			n, e := strconv.ParseFloat(text, 64)
			if e == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
				out[key][int64(stamp)] = n
			}
		}
	}
	return out, nil
}
func sample(data map[string]series, id, component string, at int64) (float64, bool) {
	n, ok := data[id+"/"+component][at]
	return n, ok
}
func podKey(p pod, id string) string {
	if p.Namespace != "" && p.Name != "" {
		return p.Namespace + "/" + p.Name + "/" + id
	}
	return id
}
func (s Service) buildTrend(state preparedState, r row, metric string, used, config map[string]series, from, to time.Time) Trend {
	kind := metric
	if metric == "storage" {
		kind = "ephemeral"
		if state.Type == "node" {
			kind = "local_filesystem"
		}
	}
	tr := Trend{Kind: kind, Unit: "bytes", State: "no_data", Basis: "usage", Points: []Point{}}
	if metric == "cpu" {
		tr.Unit = "cores"
	}
	b := r.Budgets[metric]
	tr.CurrentRequest, tr.CurrentLimit, tr.PartialConfiguration = b.Request, b.Limit, b.Partial
	if !state.Complete {
		tr.CurrentRequest = nil
		tr.CurrentLimit = nil
	}
	if n := r.Capacity[metric]; n > 0 {
		tr.CurrentCapacity = number(n)
	}
	if n := r.Allocatable[metric]; n > 0 {
		tr.Allocatable = number(n)
	}
	members := []pod{}
	for _, p := range state.Pods {
		if slices.Contains(p.Parents, r.UID) {
			members = append(members, p)
		}
	}
	anyValue, anyFresh := false, false
	for at := from; !at.After(to); at = at.Add(30 * time.Second) {
		stamp := at.Unix()
		point := Point{Time: at}
		usedTotal, reqTotal, limTotal := 0., 0., 0.
		expected, found := 0, 0
		reqComplete, limComplete := true, true
		if state.Type == "node" {
			n, ok := sample(used, r.Name, "used", stamp)
			st, valid := sample(used, r.Name, "sample_time", stamp)
			if ok && valid && float64(stamp)-st <= 90 && st >= float64(r.Created.Unix()) && at.After(r.Created) {
				point.Value = number(n)
				if metric != "storage" {
					point.Capacity = tr.CurrentCapacity
				} else if capacity, ok := sample(used, r.Name, "capacity", stamp); ok && capacity > 0 {
					point.Capacity = number(capacity)
				}
				anyValue = true
				if stamp >= to.Unix()-60 {
					anyFresh = true
				}
			}
		} else {
			for _, p := range members {
				if at.Before(p.Created) {
					continue
				}
				expected++
				id := podKey(p, p.UID)
				n, ok := sample(used, id, "used", stamp)
				if !ok && p.Alias != "" {
					id = podKey(p, p.Alias)
					n, ok = sample(used, id, "used", stamp)
				}
				st, stOK := sample(used, id, "sample_time", stamp)
				if !ok || !stOK || float64(stamp)-st > 90 || st < float64(p.Created.Unix()) {
					continue
				}
				found++
				usedTotal += n
				rq, rOK := sample(config, podKey(p, p.UID), "request", stamp)
				rc, rcOK := sample(config, podKey(p, p.UID), "request_count", stamp)
				lm, lOK := sample(config, podKey(p, p.UID), "limit", stamp)
				lc, lcOK := sample(config, podKey(p, p.UID), "limit_count", stamp)
				if rOK && rcOK && int(rc) == p.Containers && rq > 0 {
					reqTotal += rq
				} else {
					reqComplete = false
				}
				if lOK && lcOK && int(lc) == p.Containers && lm > 0 {
					limTotal += lm
				} else {
					limComplete = false
				}
			}
			if expected > 0 && expected == found && state.Complete {
				point.Value = number(usedTotal)
				if reqComplete {
					point.Request = number(reqTotal)
				}
				if limComplete {
					point.Limit = number(limTotal)
				}
				anyValue = true
				if stamp >= to.Unix()-60 {
					anyFresh = true
				}
			}
		}
		tr.Points = append(tr.Points, point)
	}
	if anyValue {
		tr.State = "ready"
		if !anyFresh {
			tr.State = "stale"
		}
	}
	if !state.Complete && state.Type != "node" {
		tr.State = "partial"
		tr.Reason = "INCOMPLETE"
	}
	for i := len(tr.Points) - 1; i >= 0; i-- {
		p := tr.Points[i]
		if p.Value == nil {
			continue
		}
		if p.Capacity != nil {
			tr.Basis = "capacity"
		} else if p.Limit != nil {
			tr.Basis = "limit"
		} else if p.Request != nil {
			tr.Basis = "request"
		}
		break
	}
	return tr
}
