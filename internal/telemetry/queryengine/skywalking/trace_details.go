package skywalking

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

func (state *executionState) querySpans(ctx context.Context, trace traceRecord) ([]spanRecord, error) {
	key := trace.resourceID + "/" + trace.sourceID + "/" + trace.traceID
	state.mu.Lock()
	defer state.mu.Unlock()
	if cached, ok := state.spans[key]; ok {
		return cached, nil
	}
	remaining := min(state.request.Budget.MaxRelationExpansions-state.relations, state.request.Budget.MaxRows-state.rows-state.relations)
	if remaining <= 0 {
		return nil, ErrBudget
	}
	where, args := state.spanFilterScope()
	where += " AND trace_id=? AND source_id=? AND resource_id=?"
	args = append(args, trace.traceID, trace.sourceID, trace.resourceID, remaining+1)
	query := "SELECT span_id,parent_span_id,service_name,operation,status,start_time,duration_ns,attributes,events,links,resource_attributes,scope_name,span_kind,status_message,trace_state FROM (" + state.spanFacts(where) + ") ORDER BY start_time,span_id LIMIT ?"
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []spanRecord{}
	for rows.Next() {
		if len(items) == remaining {
			return nil, ErrBudget
		}
		var item spanRecord
		var attrs, resourceAttrs map[string]string
		if err := rows.Scan(&item.spanID, &item.parentSpanID, &item.serviceName, &item.operationName, &item.status, &item.startTime, &item.duration, &attrs, &item.events, &item.links, &resourceAttrs, &item.scopeName, &item.kind, &item.statusMessage, &item.traceState); err != nil {
			return nil, err
		}
		state.request.Budget.progress.ObserveEvent(item.startTime)
		encoded, err := json.Marshal(attrs)
		if err != nil {
			return nil, err
		}
		item.attributes = string(encoded)
		encoded, err = json.Marshal(resourceAttrs)
		if err != nil {
			return nil, err
		}
		item.resourceAttributes = string(encoded)
		item.resourceID, item.sourceID, item.traceID = trace.resourceID, trace.sourceID, trace.traceID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	state.relations += len(items)
	if state.spans == nil {
		state.spans = map[string][]spanRecord{}
	}
	state.spans[key] = items
	return items, nil
}

func (state *executionState) queryEdges(ctx context.Context, trace traceRecord) ([]edgeRecord, error) {
	spans, err := state.querySpans(ctx, trace)
	if err != nil {
		return nil, err
	}
	edges, _ := assembleEdges(spans)
	return edges, nil
}

// Iterative traversal is bounded by received spans, including malformed
// cycles. Missing parents remain explicit; no edge is invented from Links.
func assembleEdges(spans []spanRecord) ([]edgeRecord, bool) {
	byID := map[string]spanRecord{}
	for _, s := range spans {
		byID[s.spanID] = s
	}
	depths, cyclic := map[string]int{}, map[string]bool{}
	for _, span := range spans {
		if _, ok := depths[span.spanID]; ok {
			continue
		}
		path, visiting := []string{}, map[string]bool{}
		current := span.spanID
		base := -1
		cycle := false
		for current != "" {
			if d, ok := depths[current]; ok {
				base = d
				cycle = cyclic[current]
				break
			}
			if visiting[current] {
				cycle = true
				break
			}
			s, ok := byID[current]
			if !ok {
				break
			}
			visiting[current] = true
			path = append(path, current)
			current = s.parentSpanID
		}
		for i := len(path) - 1; i >= 0; i-- {
			base++
			depths[path[i]] = base
			cyclic[path[i]] = cycle
		}
	}
	edges := []edgeRecord{}
	hasCycle := false
	for _, span := range spans {
		if span.parentSpanID == "" {
			continue
		}
		parent, found := byID[span.parentSpanID]
		edges = append(edges, edgeRecord{parentSpanID: span.parentSpanID, childSpanID: span.spanID, parentService: parent.serviceName, childService: span.serviceName, depth: uint32(depths[span.spanID]), missingParent: !found, cycle: cyclic[span.spanID]})
		hasCycle = hasCycle || cyclic[span.spanID]
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].depth != edges[j].depth {
			return edges[i].depth < edges[j].depth
		}
		return edges[i].childSpanID < edges[j].childSpanID
	})
	return edges, hasCycle
}

func (r *traceResolver) Completeness(ctx context.Context) (string, error) {
	spans, err := r.state.querySpans(ctx, r.item)
	if err != nil {
		return "", r.state.failed(err)
	}
	_, cycle := assembleEdges(spans)
	if cycle || r.item.rootCount > 1 {
		return "invalid_graph", nil
	}
	if !r.item.rootPresent || r.item.missingParents > 0 {
		return "missing_spans", nil
	}
	// Parent consistency cannot establish that an upstream sampler delivered
	// every span; it must never be described as verified complete telemetry.
	return "observed_consistent", nil
}

func (state *executionState) failed(err error) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.lastError = err
	return err
}

func validateTraceIdentity(value *string) error {
	if value == nil {
		return nil
	}
	if !validUUID(*value) {
		return fmt.Errorf("invalid trace source/resource identity")
	}
	return nil
}
