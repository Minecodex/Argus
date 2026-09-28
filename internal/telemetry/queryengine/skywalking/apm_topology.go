package skywalking

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
)

func (*rootResolver) QueryAPMTopology(ctx context.Context, args apmArgs) (*topologyResult, error) {
	if err := validateAPMArgs("topology", args, 0); err != nil {
		return nil, err
	}
	if ctx.Value(inputValidationKey{}) == true {
		return &topologyResult{nodes: []*topologyNode{}, edges: []*topologyEdge{}}, nil
	}
	state, err := executionStateFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if !state.request.End.After(state.request.Start) || state.request.End.Sub(state.request.Start) > 7*24*time.Hour {
		return nil, state.failed(fmt.Errorf("topology requires a time window within seven days"))
	}
	spans, err := state.loadTopologySpans(ctx, args)
	if err != nil {
		return nil, state.failed(err)
	}
	result, err := assembleTopology(ctx, spans, args, state.request.Start, state.request.End)
	if err != nil {
		return nil, state.failed(err)
	}
	if err := state.recordRows(len(result.nodes) + len(result.edges) + 1); err != nil {
		return nil, state.failed(err)
	}
	state.apmWarning(result.status)
	return result, nil
}

func (state *executionState) loadTopologySpans(ctx context.Context, args apmArgs) ([]topologySpan, error) {
	where, values := state.spanFilterScope()
	for _, filter := range []struct {
		column string
		value  *string
	}{{"source_id", args.SourceID}, {"resource_id", args.ResourceID}} {
		if value := stringValue(filter.value); value != "" {
			where += " AND " + filter.column + "=?"
			values = append(values, value)
		}
	}
	state.mu.Lock()
	remaining := min(state.request.Budget.MaxRelationExpansions-state.relations, state.request.Budget.MaxRows-state.rows-state.relations-1)
	state.mu.Unlock()
	if remaining <= 0 {
		return nil, ErrBudget
	}
	attributes, filterValues := apmAttributeSQL(args.Filters)
	values = append(values, filterValues...)
	query := "SELECT resource_id,source_id,source_type,trace_id,span_id,parent_span_id,service_name,operation,status,span_kind,duration_ns,resource_attributes['service.instance.id'],resource_attributes['service.instance.name'],start_time FROM (" + state.spanFacts(where) + ") WHERE 1" + attributes + " ORDER BY resource_id,source_id,trace_id,span_id LIMIT ?"
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), query, append(values, remaining+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spans := []topologySpan{}
	for rows.Next() {
		if len(spans) == remaining {
			return nil, ErrBudget
		}
		var span topologySpan
		var sampleTime time.Time
		var resource, source uuid.UUID
		if err := rows.Scan(&resource, &source, &span.sourceType, &span.trace, &span.id, &span.parent, &span.service, &span.operation, &span.status, &span.kind, &span.duration, &span.instanceID, &span.instanceName, &sampleTime); err != nil {
			return nil, err
		}
		state.request.Budget.progress.ObserveEvent(sampleTime)
		span.resource, span.source = resource.String(), source.String()
		spans = append(spans, span)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	state.mu.Lock()
	state.relations += len(spans)
	state.mu.Unlock()
	return spans, nil
}

func assembleTopology(ctx context.Context, spans []topologySpan, args apmArgs, start, end time.Time) (*topologyResult, error) {
	r := &topologyResult{status: "available", start: start, end: end, nodes: []*topologyNode{}, edges: []*topologyEdge{}}
	parents, missing, ambiguous, err := resolveSpanParents(ctx, spans)
	if err != nil {
		return nil, err
	}
	byID := map[string]topologySpan{}
	for _, span := range spans {
		byID[span.key()] = span
	}
	cyclic := topologyCycles(parents)
	nodes, edges := map[string]*topologyNode{}, map[string]*topologyEdge{}
	for index, span := range spans {
		if index%256 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if args.ServiceInstanceName != nil && span.instanceID != *args.ServiceInstanceName && span.instanceName != *args.ServiceInstanceName {
			continue
		}
		if args.OperationName != nil && span.operation != *args.OperationName {
			continue
		}
		parent, hasParent := byID[parents[span.key()]]
		if args.ServiceName != nil && span.service != *args.ServiceName && (!hasParent || parent.service != *args.ServiceName) {
			continue
		}
		r.coverage.observed++
		if span.source == uuid.Nil.String() || span.sourceType == "unknown" || span.sourceType == "" {
			r.coverage.unknownSource++
		}
		if span.service == "" {
			r.coverage.missingService++
		}
		if hasParent && parent.service == "" && span.service != "" {
			r.coverage.missingService++
		}
		if missing[span.key()] {
			r.coverage.missing++
		}
		if ambiguous[span.key()] {
			r.coverage.ambiguous++
		}
		if cyclic[span.key()] {
			r.coverage.cyclic++
		}
		if span.service == "" {
			continue
		}
		childNode := span.node()
		nodes[childNode.id] = childNode
		if !hasParent || parent.service == "" || cyclic[span.key()] {
			continue
		}
		parentNode := parent.node()
		nodes[parentNode.id] = parentNode
		if parentNode.id == childNode.id {
			continue
		}
		key := parentNode.id + "/" + childNode.id
		edge := edges[key]
		if edge == nil {
			edge = &topologyEdge{from: parentNode.id, to: childNode.id, stats: apmRowResolver{seconds: end.Sub(start).Seconds()}}
			edges[key] = edge
		}
		edge.count++
		if span.kind == 2 || span.kind == 5 {
			edge.stats.samples++
			if span.status == "error" {
				edge.stats.errors++
			}
			edge.durations = append(edge.durations, float64(span.duration)/1e6)
		}
	}
	keys := []string{}
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if args.Limit != nil && len(r.edges) == int(*args.Limit) {
			r.limited = true
			break
		}
		e := edges[key]
		slices.Sort(e.durations)
		if len(e.durations) > 0 {
			for _, d := range e.durations {
				e.stats.mean += d
			}
			e.stats.mean /= float64(len(e.durations))
			for _, p := range []float64{0.5, 0.95, 0.99} {
				e.stats.quantiles = append(e.stats.quantiles, e.durations[int(math.Ceil(float64(len(e.durations))*p))-1])
			}
		}
		r.edges = append(r.edges, e)
	}
	for _, node := range nodes {
		r.nodes = append(r.nodes, node)
	}
	sort.Slice(r.nodes, func(i, j int) bool { return r.nodes[i].id < r.nodes[j].id })
	if r.coverage.observed == 0 {
		r.status = "no_data"
	} else if r.coverage.missing+r.coverage.ambiguous+r.coverage.cyclic+r.coverage.missingService+r.coverage.unknownSource > 0 {
		r.status = "incomplete_observations"
	}
	return r, nil
}

func topologyCycles(parents map[string]string) map[string]bool {
	finished, cyclic := map[string]bool{}, map[string]bool{}
	for start := range parents {
		if finished[start] {
			continue
		}
		path, seen := []string{}, map[string]bool{}
		current := start
		cycle := false
		for current != "" {
			if finished[current] {
				cycle = cyclic[current]
				break
			}
			if seen[current] {
				cycle = true
				break
			}
			seen[current] = true
			path = append(path, current)
			current = parents[current]
		}
		for _, key := range path {
			finished[key] = true
			cyclic[key] = cycle
		}
	}
	return cyclic
}

func resolveSpanParents(ctx context.Context, spans []topologySpan) (map[string]string, map[string]bool, map[string]bool, error) {
	type parentChoice struct {
		span  topologySpan
		count int
	}
	byID, byParent := map[string]topologySpan{}, map[string]parentChoice{}
	for index, span := range spans {
		if index%256 == 0 && ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
		byID[span.key()] = span
		if span.source != uuid.Nil.String() && span.sourceType != "" && span.sourceType != "unknown" {
			key := span.sourceType + "/" + span.trace + "/" + span.id
			choice := byParent[key]
			choice.span = span
			choice.count++
			byParent[key] = choice
		}
	}
	parents := map[string]string{}
	missing, ambiguous := map[string]bool{}, map[string]bool{}
	for index, span := range spans {
		if index%256 == 0 && ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
		if span.parent == "" {
			continue
		}
		// Prefer the same installation/resource; otherwise require exactly one
		// authorized parent from the same known receiver type.
		local := span.resource + "/" + span.source + "/" + span.trace + "/" + span.parent
		if _, ok := byID[local]; ok {
			parents[span.key()] = local
			continue
		}
		choice := parentChoice{}
		if span.source != uuid.Nil.String() && span.sourceType != "" && span.sourceType != "unknown" {
			choice = byParent[span.sourceType+"/"+span.parentKey()]
		}
		switch choice.count {
		case 0:
			missing[span.key()] = true
		case 1:
			parents[span.key()] = choice.span.key()
		default:
			ambiguous[span.key()] = true
		}
	}

	return parents, missing, ambiguous, nil
}
