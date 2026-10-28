package modelmetrics

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

func (s Service) QueryModelGovernance(ctx context.Context, r Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	access, err := Resolve(ctx, s.Store, r)
	if err != nil {
		return Response{}, err
	}
	for _, id := range r.PanelIDs {
		if !slices.ContainsFunc(Panels(), func(p Panel) bool { return p.ID == id }) {
			return Response{}, ErrInvalid
		}
	}
	data, err := s.read(ctx, r, access)
	if err != nil {
		return Response{}, err
	}
	data.Response.CachedUsageComplete = true
	var latestCall *time.Time
	for _, c := range data.Calls {
		if latestCall == nil || c.CompletedAt.After(*latestCall) {
			v := c.CompletedAt
			latestCall = &v
		}
		if c.CachedSource != "provider" || c.InputSource != "provider" || c.Cached > c.Input {
			data.Response.CachedUsageComplete = false
		}
		if c.InputSource != "provider" || c.OutputSource != "provider" {
			data.Response.UsageComplete = false
		}
		if c.InputSource == "estimated" {
			data.Response.EstimatedInput += c.Input
		}
		if c.OutputSource == "estimated" {
			data.Response.EstimatedOutput += c.Output
		}
	}
	if r.CandidatesOnly {
		current, err := Resolve(ctx, s.Store, r)
		if err != nil || current.Hash != access.Hash {
			return Response{}, ErrDenied
		}
		return data.Response, nil
	}
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 1000000, Timeout: 20 * time.Second, LookbackDelta: time.Minute, EnableAtModifier: true, Parser: parser.NewParser(parser.Options{})})
	now := time.Now().UTC()
	end := access.To
	if end.After(now) {
		end = now
	}
	totals, err := data.metrics(data.Calls, end, true)
	if err != nil {
		return Response{}, err
	}
	location, _ := time.LoadLocation(access.Timezone)
	buckets := map[string][]Call{}
	for _, c := range data.Calls {
		day := c.CompletedAt.In(location).Format("2006-01-02")
		buckets[day] = append(buckets[day], c)
	}
	for _, panel := range Panels() {
		if len(r.PanelIDs) > 0 && !slices.Contains(r.PanelIDs, panel.ID) {
			continue
		}
		result := queryengine.Result{Language: queryengine.LanguagePromQL, ResultType: "vector", Meta: queryengine.QueryMeta{Engine: "prometheus-promql", EngineVersion: "v0.314.0", QueryCompletedAt: &now, SampleTimeBasis: "model_call_completed_at", IngestionStatus: "ready", LatestSampleAt: latestCall, Warnings: []string{}}}
		if panel.ID == "quota" {
			result.Meta.SampleTimeBasis = "internal_projection_applied_at"
			result.Meta.LatestSampleAt = data.Response.ProjectedAt
		}
		if data.Response.Pending > 0 {
			result.Meta.IngestionStatus = "delayed"
			result.Meta.Partial = true
			result.Meta.Warnings = append(result.Meta.Warnings, "MODEL_METRIC_PROJECTION_PENDING")
		}
		if !data.Response.UsageComplete {
			result.Meta.Warnings = append(result.Meta.Warnings, "MODEL_USAGE_INCOMPLETE")
		}
		if !data.Response.CachedUsageComplete {
			result.Meta.Warnings = append(result.Meta.Warnings, "MODEL_CACHE_USAGE_INCOMPLETE")
		}
		if panel.ID == "tool_failures" || panel.ID == "structured_failures" {
			data.Response.Panels = append(data.Response.Panels, PanelResult{ID: panel.ID, Result: result, Unavailable: true})
			continue
		}
		if !panel.Trend {
			result.Data, err = evaluate(ctx, engine, totals, panel.Expression, end)
		} else {
			result.ResultType = "matrix"
			series := map[string]*promql.Series{}
			for day := access.From.In(location); day.Before(end); day = day.AddDate(0, 0, 1) {
				rows := buckets[day.Format("2006-01-02")]
				if len(rows) == 0 {
					continue
				}
				values, e := data.metrics(rows, day.UTC(), false)
				if e != nil {
					return Response{}, e
				}
				vector, e := evaluate(ctx, engine, values, panel.Expression, day.UTC())
				if e != nil {
					return Response{}, e
				}
				for _, point := range vector {
					key := point.Metric.String()
					if series[key] == nil {
						series[key] = &promql.Series{Metric: point.Metric}
					}
					series[key].Floats = append(series[key].Floats, promql.FPoint{T: day.UnixMilli(), F: point.F})
				}
			}
			matrix := promql.Matrix{}
			for _, row := range series {
				matrix = append(matrix, *row)
			}
			result.Data = matrix
		}
		if err != nil {
			return Response{}, err
		}
		data.Response.Panels = append(data.Response.Panels, PanelResult{ID: panel.ID, Result: result})
	}
	// No cache is used in this private domain. Recheck membership and current
	// department before returning even when authorization changed during a query.
	current, err := Resolve(ctx, s.Store, r)
	if err != nil || current.Hash != access.Hash {
		return Response{}, ErrDenied
	}
	return data.Response, nil
}

func evaluate(ctx context.Context, engine *promql.Engine, source metricQueryable, expression string, at time.Time) (promql.Vector, error) {
	query, err := engine.NewInstantQuery(ctx, source, promql.NewPrometheusQueryOpts(false, time.Minute), expression, at)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	result := query.Exec(ctx)
	if result.Err != nil {
		return nil, result.Err
	}
	vector, ok := result.Value.(promql.Vector)
	if !ok {
		return nil, ErrInvalid
	}
	copy := make(promql.Vector, 0, len(vector))
	for _, v := range vector {
		if !math.IsNaN(v.F) && !math.IsInf(v.F, 0) {
			copy = append(copy, v)
		}
	}
	return copy, nil
}
