package modelmetrics

import (
	"context"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

type metricSeries struct {
	labels labels.Labels
	chunk  *chunkenc.XORChunk
}

func (s *metricSeries) Labels() labels.Labels                           { return s.labels }
func (s *metricSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator { return s.chunk.Iterator(it) }

type metricSet struct {
	items []storage.Series
	i     int
}

func (s *metricSet) Next() bool                        { s.i++; return s.i < len(s.items) }
func (s *metricSet) At() storage.Series                { return s.items[s.i] }
func (s *metricSet) Err() error                        { return nil }
func (s *metricSet) Warnings() annotations.Annotations { return nil }

type metricQueryable struct{ items []storage.Series }

func (q metricQueryable) Querier(_, _ int64) (storage.Querier, error) { return q, nil }
func (q metricQueryable) Close() error                                { return nil }
func (q metricQueryable) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, ErrDenied
}
func (q metricQueryable) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, ErrDenied
}
func (q metricQueryable) Select(_ context.Context, _ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	result := &metricSet{i: -1}
	for _, series := range q.items {
		match := true
		for _, matcher := range matchers {
			if !matcher.Matches(series.Labels().Get(matcher.Name)) {
				match = false
				break
			}
		}
		if match {
			result.items = append(result.items, series)
		}
	}
	sort.Slice(result.items, func(i, j int) bool { return labels.Compare(result.items[i].Labels(), result.items[j].Labels()) < 0 })
	return result
}
func (q *metricQueryable) add(name string, value float64, at time.Time, dims map[string]string) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	b := labels.NewBuilder(labels.EmptyLabels())
	b.Set("__name__", name)
	for key, value := range dims {
		b.Set(key, value)
	}
	chunk := chunkenc.NewXORChunk()
	app, _ := chunk.Appender()
	app.Append(0, at.UnixMilli(), value)
	q.items = append(q.items, &metricSeries{labels: b.Labels(), chunk: chunk})
}

type totals struct {
	c                                                                                                 Call
	Requests, Successes, Errors, Cancelled, Compactions, Input, Output, Cached, Latency, LatencyCount float64
	Amount                                                                                            float64
	InputCount, OutputCount, CachedCount                                                              int
}

func (d dataset) metrics(calls []Call, at time.Time, includeQuota bool) (metricQueryable, error) {
	q := metricQueryable{}
	groups := map[string]*totals{}
	latencies := []int64{}
	quotaUsed := map[string]float64{}
	name := func(id string) string {
		if v := d.Names[id]; v != "" {
			return v
		}
		return id
	}
	for _, c := range calls {
		key := c.ModelID.String() + c.DepartmentID.String() + c.UserID.String()
		v := groups[key]
		if v == nil {
			v = &totals{c: c}
			groups[key] = v
		}
		if len(groups) > 5000 {
			return q, ErrBudget
		}
		v.Requests++
		if c.Status == "succeeded" {
			v.Successes++
		}
		if c.Status == "failed" {
			v.Errors++
		}
		if c.Status == "cancelled" {
			v.Cancelled++
		}
		if c.CallKind == "compaction" {
			v.Compactions++
		}
		if c.InputSource == "provider" {
			v.Input += float64(c.Input)
			v.InputCount++
		}
		if c.OutputSource == "provider" {
			v.Output += float64(c.Output)
			v.OutputCount++
		}
		if c.InputSource == "provider" && c.CachedSource == "provider" && c.Cached <= c.Input {
			v.Cached += float64(c.Cached)
			v.CachedCount++
		}
		amount, err := strconv.ParseFloat(c.Amount, 64)
		if err != nil {
			return q, err
		}
		v.Amount += amount
		if c.Dispatched && c.LatencyMS > 0 {
			v.Latency += float64(c.LatencyMS)
			v.LatencyCount++
			latencies = append(latencies, c.LatencyMS)
		}
	}
	// Empty calls deliberately produce no measurements, including no fake 100%.
	for _, v := range groups {
		dims := map[string]string{"model_id": v.c.ModelID.String(), "model": name(v.c.ModelID.String()), "department_id": v.c.DepartmentID.String(), "department": name(v.c.DepartmentID.String()), "user_id": v.c.UserID.String(), "user": name(v.c.UserID.String())}
		for metric, value := range map[string]float64{"requests": v.Requests, "successes": v.Successes, "errors": v.Errors, "cancelled": v.Cancelled, "compactions": v.Compactions, "input_tokens": v.Input, "output_tokens": v.Output, "cached_tokens": v.Cached, "amount": v.Amount, "latency_ms": v.Latency, "latency_count": v.LatencyCount} {
			if metric == "input_tokens" && v.InputCount == 0 || metric == "output_tokens" && v.OutputCount == 0 || metric == "cached_tokens" && v.CachedCount == 0 {
				continue
			}
			q.add("argus_model_"+metric, value, at, dims)
		}
	}
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		q.add("argus_model_latency_p95_ms", float64(latencies[int(math.Ceil(.95*float64(len(latencies))))-1]), at, nil)
	}
	if includeQuota {
		for _, reservation := range d.Reservations {
			text := reservation.Reserved
			if reservation.Status == "settled" {
				text = reservation.Settled
			} else if reservation.Status != "active" {
				continue
			}
			amount, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return q, err
			}
			quotaUsed[reservation.ModelID.String()+reservation.DepartmentID.String()] += amount
		}
		for _, quota := range d.Quotas {
			if quota.SubjectType != "department" {
				continue
			}
			limit, err := strconv.ParseFloat(quota.Amount, 64)
			if err != nil {
				return q, err
			}
			if limit <= 0 {
				continue
			}
			dims := map[string]string{"model": name(quota.ModelID.String()), "model_id": quota.ModelID.String(), "department": name(quota.SubjectID.String()), "department_id": quota.SubjectID.String()}
			q.add("argus_model_quota_used", quotaUsed[quota.ModelID.String()+quota.SubjectID.String()], at, dims)
			q.add("argus_model_quota_limit", limit, at, dims)
		}
	}
	return q, nil
}
