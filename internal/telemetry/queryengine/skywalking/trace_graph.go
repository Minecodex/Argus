package skywalking

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

type traceGraphArgs struct {
	TraceID, SourceID, ResourceID string
	Filters                       *[]apmAttributeFilter
}
type traceGraphResolver struct {
	traceID, status              string
	missing, ambiguous, excluded int
	spans                        []*spanResolver
	edges                        []*traceGraphEdgeResolver
}

func (r *traceGraphResolver) TraceID() string                  { return r.traceID }
func (r *traceGraphResolver) Completeness() string             { return r.status }
func (r *traceGraphResolver) MissingParentCount() int32        { return boundedInt32(uint64(r.missing)) }
func (r *traceGraphResolver) AmbiguousParentCount() int32      { return boundedInt32(uint64(r.ambiguous)) }
func (r *traceGraphResolver) ExcludedSpanCount() int32         { return boundedInt32(uint64(r.excluded)) }
func (r *traceGraphResolver) Spans() []*spanResolver           { return r.spans }
func (r *traceGraphResolver) Edges() []*traceGraphEdgeResolver { return r.edges }

type traceGraphEdgeResolver struct {
	parentID, childID, parentSource, childSource, parentResource, childResource string
	missing, cycle                                                              bool
}

func (r *traceGraphEdgeResolver) ParentSpanID() string     { return r.parentID }
func (r *traceGraphEdgeResolver) ChildSpanID() string      { return r.childID }
func (r *traceGraphEdgeResolver) ParentSourceID() string   { return r.parentSource }
func (r *traceGraphEdgeResolver) ChildSourceID() string    { return r.childSource }
func (r *traceGraphEdgeResolver) ParentResourceID() string { return r.parentResource }
func (r *traceGraphEdgeResolver) ChildResourceID() string  { return r.childResource }
func (r *traceGraphEdgeResolver) MissingParent() bool      { return r.missing }
func (r *traceGraphEdgeResolver) Cycle() bool              { return r.cycle }

func (*rootResolver) QueryTraceGraph(ctx context.Context, args traceGraphArgs) (*traceGraphResolver, error) {
	if len(args.TraceID) == 0 || len(args.TraceID) > 256 {
		return nil, fmt.Errorf("invalid trace identity")
	}
	if err := validateTraceIdentity(&args.SourceID); err != nil {
		return nil, err
	}
	if err := validateTraceIdentity(&args.ResourceID); err != nil {
		return nil, err
	}
	if err := validateAPMAttributes(args.Filters); err != nil {
		return nil, err
	}
	if ctx.Value(inputValidationKey{}) == true {
		return nil, nil
	}
	state, err := executionStateFromContext(ctx)
	if err != nil {
		return nil, err
	}
	args.SourceID, args.ResourceID = uuid.MustParse(args.SourceID).String(), uuid.MustParse(args.ResourceID).String()
	result, err := state.queryTraceGraph(ctx, args)
	if err != nil {
		return nil, state.failed(err)
	}
	return result, nil
}

func (state *executionState) queryTraceGraph(ctx context.Context, args traceGraphArgs) (*traceGraphResolver, error) {
	where, values := state.spanFilterScope()
	where += " AND trace_id=?"
	values = append(values, args.TraceID)
	filter, filterArgs := apmAttributeSQL(args.Filters)
	values = append(values, filterArgs...)
	state.mu.Lock()
	remaining := min(state.request.Budget.MaxRelationExpansions-state.relations, state.request.Budget.MaxRows-state.rows-state.relations-1)
	state.mu.Unlock()
	if remaining <= 0 {
		return nil, ErrBudget
	}
	query := "SELECT resource_id,source_id,source_type,span_id,parent_span_id,service_name,operation,status,start_time,duration_ns,attributes,events,links,resource_attributes,scope_name,span_kind,status_message,trace_state FROM (" + state.spanFacts(where) + ") WHERE 1" + filter + " ORDER BY start_time,resource_id,source_id,span_id LIMIT ?"
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), query, append(values, remaining+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := []topologySpan{}
	records := map[string]spanRecord{}
	for rows.Next() {
		if len(facts) == remaining {
			return nil, ErrBudget
		}
		var record spanRecord
		var source, resource uuid.UUID
		var sourceType string
		var attrs, resourceAttrs map[string]string
		if err := rows.Scan(&resource, &source, &sourceType, &record.spanID, &record.parentSpanID, &record.serviceName, &record.operationName, &record.status, &record.startTime, &record.duration, &attrs, &record.events, &record.links, &resourceAttrs, &record.scopeName, &record.kind, &record.statusMessage, &record.traceState); err != nil {
			return nil, err
		}
		state.request.Budget.progress.ObserveEvent(record.startTime)
		record.resourceID, record.sourceID, record.traceID = resource.String(), source.String(), args.TraceID
		encoded, _ := json.Marshal(attrs)
		record.attributes = string(encoded)
		encoded, _ = json.Marshal(resourceAttrs)
		record.resourceAttributes = string(encoded)
		fact := topologySpan{resource: record.resourceID, source: record.sourceID, sourceType: sourceType, trace: args.TraceID, id: record.spanID, parent: record.parentSpanID, service: record.serviceName}
		facts = append(facts, fact)
		records[fact.key()] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	state.mu.Lock()
	state.relations += len(facts)
	state.mu.Unlock()
	parents, missing, ambiguous, err := resolveSpanParents(ctx, facts)
	if err != nil {
		return nil, err
	}
	connected := map[string]bool{}
	queue := []string{}
	neighbors := map[string][]string{}
	for child, parent := range parents {
		neighbors[child] = append(neighbors[child], parent)
		neighbors[parent] = append(neighbors[parent], child)
	}
	for _, fact := range facts {
		if fact.resource == args.ResourceID && fact.source == args.SourceID {
			connected[fact.key()] = true
			queue = append(queue, fact.key())
		}
	}
	if len(queue) == 0 {
		return nil, nil
	}
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		key := queue[0]
		queue = queue[1:]
		for _, next := range neighbors[key] {
			if !connected[next] {
				connected[next] = true
				queue = append(queue, next)
			}
		}
	}
	cycles := topologyCycles(parents)
	r := &traceGraphResolver{traceID: args.TraceID, status: "observed_connected", spans: []*spanResolver{}, edges: []*traceGraphEdgeResolver{}, excluded: len(facts) - len(connected)}
	rootCount := 0
	for _, fact := range facts {
		key := fact.key()
		if !connected[key] {
			continue
		}
		record := records[key]
		r.spans = append(r.spans, &spanResolver{item: record})
		if record.parentSpanID == "" {
			rootCount++
			continue
		}
		if missing[key] {
			r.missing++
		}
		if ambiguous[key] {
			r.ambiguous++
		}
		parent, found := records[parents[key]]
		r.edges = append(r.edges, &traceGraphEdgeResolver{parentID: fact.parent, childID: fact.id, parentSource: parent.sourceID, childSource: fact.source, parentResource: parent.resourceID, childResource: fact.resource, missing: !found, cycle: cycles[key]})
		if cycles[key] {
			r.status = "invalid_graph"
		}
	}
	if r.status != "invalid_graph" {
		switch {
		case rootCount > 1:
			r.status = "invalid_graph"
		case r.ambiguous > 0 || r.excluded > 0:
			r.status = "ambiguous_fragments"
		case rootCount == 0 || r.missing > 0:
			r.status = "missing_spans"
		}
	}
	sort.Slice(r.edges, func(i, j int) bool {
		a, b := r.edges[i], r.edges[j]
		return a.childResource+"/"+a.childSource+"/"+a.childID < b.childResource+"/"+b.childSource+"/"+b.childID
	})
	if err := state.recordRows(1); err != nil {
		return nil, err
	}
	return r, nil
}
