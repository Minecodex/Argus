package skywalking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"math"
	"strings"
	"sync"
	"time"

	queryerrors "github.com/graph-gophers/graphql-go/errors"
)

type executionStateKey struct{}

type traceTables struct {
	spans string
}

type executionState struct {
	partial   bool
	warnings  []string
	rows      int
	engine    Engine
	request   Request
	tables    traceTables
	mu        sync.Mutex
	spans     map[string][]spanRecord
	relations int
	lastError error
}

func withExecutionState(ctx context.Context, state *executionState) context.Context {
	return context.WithValue(ctx, executionStateKey{}, state)
}

func executionStateFromContext(ctx context.Context) (*executionState, error) {
	state, ok := ctx.Value(executionStateKey{}).(*executionState)
	if !ok || state == nil {
		return nil, fmt.Errorf("trace graphql execution state unavailable")
	}
	return state, nil
}

type rootResolver struct{}

type traceTagInput struct {
	Key   string
	Value string
}

type basicTraceArgs struct {
	ServiceName         *string
	ServiceInstanceName *string
	Tags                *[]traceTagInput
	DurationMin         *float64
	DurationMax         *float64
	Order               *string
	PageNum             *int32
	PageSize            *int32
}

type namedTraceArgs struct {
	ServiceName         *string
	ServiceInstanceName *string
	OperationName       *string
	Tags                *[]traceTagInput
	DurationMin         *float64
	DurationMax         *float64
	Order               *string
	PageNum             *int32
	PageSize            *int32
}

type statusTraceArgs struct {
	SourceID, ResourceID, TraceID, OperationName *string
	Filters                                      *[]apmAttributeFilter
	ServiceName                                  *string
	ServiceInstanceName                          *string
	Status                                       *string
	Tags                                         *[]traceTagInput
	DurationMin                                  *float64
	DurationMax                                  *float64
	Order                                        *string
	PageNum                                      *int32
	PageSize                                     *int32
}

type traceFilter struct {
	sourceID, resourceID, traceID                           *string
	filters                                                 *[]apmAttributeFilter
	serviceName, serviceInstanceName, operationName, status *string
	tags                                                    *[]traceTagInput
	durationMin, durationMax                                *float64
	order                                                   *string
	pageNum, pageSize                                       *int32
}

func (*rootResolver) QueryBasicTraces(ctx context.Context, args basicTraceArgs) (*traceQueryResultResolver, error) {
	return resolveTracePage(ctx, traceFilter{serviceName: args.ServiceName, serviceInstanceName: args.ServiceInstanceName, tags: args.Tags, durationMin: args.DurationMin, durationMax: args.DurationMax, order: args.Order, pageNum: args.PageNum, pageSize: args.PageSize})
}

func (*rootResolver) QueryBasicTracesByName(ctx context.Context, args namedTraceArgs) (*traceQueryResultResolver, error) {
	return resolveTracePage(ctx, traceFilter{serviceName: args.ServiceName, serviceInstanceName: args.ServiceInstanceName, operationName: args.OperationName, tags: args.Tags, durationMin: args.DurationMin, durationMax: args.DurationMax, order: args.Order, pageNum: args.PageNum, pageSize: args.PageSize})
}

func (*rootResolver) QueryTraces(ctx context.Context, args statusTraceArgs) (*traceQueryResultResolver, error) {
	return resolveTracePage(ctx, traceFilter{sourceID: args.SourceID, resourceID: args.ResourceID, traceID: args.TraceID, operationName: args.OperationName, filters: args.Filters, serviceName: args.ServiceName, serviceInstanceName: args.ServiceInstanceName, status: args.Status, tags: args.Tags, durationMin: args.DurationMin, durationMax: args.DurationMax, order: args.Order, pageNum: args.PageNum, pageSize: args.PageSize})
}

func (*rootResolver) QueryTrace(ctx context.Context, args struct {
	TraceID    string
	SourceID   *string
	ResourceID *string
}) (*traceResolver, error) {
	if err := validateTraceIdentity(args.SourceID); err != nil {
		return nil, err
	}
	if err := validateTraceIdentity(args.ResourceID); err != nil {
		return nil, err
	}
	if args.TraceID == "" || len(args.TraceID) > 256 {
		return nil, fmt.Errorf("invalid trace id")
	}
	if ctx.Value(inputValidationKey{}) == true {
		return nil, nil
	}
	state, err := executionStateFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if args.TraceID == "" || len(args.TraceID) > 256 {
		return nil, fmt.Errorf("invalid trace id")
	}
	item, err := state.queryTrace(ctx, args.TraceID, stringValue(args.SourceID), stringValue(args.ResourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, state.failed(err)
	}
	return &traceResolver{state: state, item: item}, nil
}

func resolveTracePage(ctx context.Context, filter traceFilter) (*traceQueryResultResolver, error) {
	if err := validateTraceIdentity(filter.sourceID); err != nil {
		return nil, err
	}
	if err := validateTraceIdentity(filter.resourceID); err != nil {
		return nil, err
	}
	if err := validateAPMAttributes(filter.filters); err != nil {
		return nil, err
	}
	if value := strings.ToLower(stringValue(filter.status)); filter.status != nil && value != "ok" && value != "error" && value != "unset" {
		return nil, fmt.Errorf("unsupported trace status")
	}
	if ctx.Value(inputValidationKey{}) == true {
		if filter.durationMin != nil && *filter.durationMin < 0 || filter.durationMax != nil && *filter.durationMax < 0 || filter.durationMin != nil && filter.durationMax != nil && *filter.durationMin > *filter.durationMax {
			return nil, fmt.Errorf("invalid trace duration range")
		}
		if filter.pageSize != nil && (*filter.pageSize < 1 || *filter.pageSize > 100000) || filter.pageNum != nil && (*filter.pageNum < 1 || *filter.pageNum > 1000000) {
			return nil, fmt.Errorf("trace pagination exceeds hard budget")
		}
		if order := strings.ToUpper(stringValue(filter.order)); order != "" && order != "START_TIME_ASC" && order != "START_TIME_DESC" {
			return nil, fmt.Errorf("unsupported trace order")
		}
		if filter.tags != nil {
			for _, tag := range *filter.tags {
				if tag.Key == "" || len(tag.Key) > 128 || len(tag.Value) > 1024 {
					return nil, fmt.Errorf("invalid trace tag")
				}
			}
		}
		return &traceQueryResultResolver{traces: []*traceResolver{}}, nil
	}
	state, err := executionStateFromContext(ctx)
	if err != nil {
		return nil, err
	}
	total, items, err := state.queryTracePage(ctx, filter)
	if err != nil {
		return nil, state.failed(err)
	}
	resolvers := make([]*traceResolver, 0, len(items))
	for _, item := range items {
		resolvers = append(resolvers, &traceResolver{state: state, item: item})
	}
	return &traceQueryResultResolver{total: boundedInt32(total), traces: resolvers}, nil
}

type traceQueryResultResolver struct {
	total  int32
	traces []*traceResolver
}

func (r *traceQueryResultResolver) Total() int32             { return r.total }
func (r *traceQueryResultResolver) Traces() []*traceResolver { return r.traces }

type traceRecord struct {
	latestSampleTime                            time.Time
	rootCount                                   uint32
	resourceID, sourceID                        string
	rootPresent                                 bool
	missingParents                              uint32
	traceID, rootService, rootOperation, status string
	startTime                                   time.Time
	duration                                    uint64
	spanCount, errorCount                       uint32
}

type traceResolver struct {
	state *executionState
	item  traceRecord
}

func (r *traceResolver) SourceID() string   { return r.item.sourceID }
func (r *traceResolver) ResourceID() string { return r.item.resourceID }
func (r *traceResolver) RootPresent() bool  { return r.item.rootPresent }
func (r *traceResolver) MissingParentCount() int32 {
	return boundedInt32(uint64(r.item.missingParents))
}
func (r *traceResolver) TraceID() string       { return r.item.traceID }
func (r *traceResolver) RootService() string   { return r.item.rootService }
func (r *traceResolver) RootOperation() string { return r.item.rootOperation }
func (r *traceResolver) StartTime() string     { return r.item.startTime.Format(time.RFC3339Nano) }
func (r *traceResolver) Duration() float64     { return float64(r.item.duration) / 1e6 }
func (r *traceResolver) SpanCount() int32      { return boundedInt32(uint64(r.item.spanCount)) }
func (r *traceResolver) ErrorCount() int32     { return boundedInt32(uint64(r.item.errorCount)) }
func (r *traceResolver) Status() string        { return r.item.status }

func (r *traceResolver) Spans(ctx context.Context) ([]*spanResolver, error) {
	items, err := r.state.querySpans(ctx, r.item)
	if err != nil {
		return nil, r.state.failed(err)
	}
	result := make([]*spanResolver, 0, len(items))
	for _, item := range items {
		result = append(result, &spanResolver{item: item})
	}
	return result, nil
}

func (r *traceResolver) Edges(ctx context.Context) ([]*edgeResolver, error) {
	items, err := r.state.queryEdges(ctx, r.item)
	if err != nil {
		return nil, r.state.failed(err)
	}
	result := make([]*edgeResolver, 0, len(items))
	for _, item := range items {
		result = append(result, &edgeResolver{item: item})
	}
	return result, nil
}

type spanRecord struct {
	traceID                                                                        string
	resourceID, sourceID, resourceAttributes, scopeName, statusMessage, traceState string
	kind                                                                           uint8
	spanID, parentSpanID, serviceName, operationName, status                       string
	startTime                                                                      time.Time
	duration                                                                       uint64
	attributes                                                                     string
	events, links                                                                  string
}

type spanResolver struct{ item spanRecord }

func (r *spanResolver) SourceID() string           { return r.item.sourceID }
func (r *spanResolver) ResourceID() string         { return r.item.resourceID }
func (r *spanResolver) ResourceAttributes() string { return r.item.resourceAttributes }
func (r *spanResolver) ScopeName() string          { return r.item.scopeName }
func (r *spanResolver) Kind() int32                { return int32(r.item.kind) }
func (r *spanResolver) StatusMessage() string      { return r.item.statusMessage }
func (r *spanResolver) TraceState() string         { return r.item.traceState }
func (r *spanResolver) SpanID() string             { return r.item.spanID }
func (r *spanResolver) ParentSpanID() string       { return r.item.parentSpanID }
func (r *spanResolver) ServiceName() string        { return r.item.serviceName }
func (r *spanResolver) OperationName() string      { return r.item.operationName }
func (r *spanResolver) Status() string             { return r.item.status }
func (r *spanResolver) StartTime() string          { return r.item.startTime.Format(time.RFC3339Nano) }
func (r *spanResolver) Duration() float64          { return float64(r.item.duration) / 1e6 }
func (r *spanResolver) Attributes() string         { return r.item.attributes }
func (r *spanResolver) Events() string             { return r.item.events }
func (r *spanResolver) Links() string              { return r.item.links }

type edgeRecord struct {
	missingParent, cycle                                   bool
	parentSpanID, childSpanID, parentService, childService string
	depth                                                  uint32
}

type edgeResolver struct{ item edgeRecord }

func (r *edgeResolver) MissingParent() bool   { return r.item.missingParent }
func (r *edgeResolver) Cycle() bool           { return r.item.cycle }
func (r *edgeResolver) ParentSpanID() string  { return r.item.parentSpanID }
func (r *edgeResolver) ChildSpanID() string   { return r.item.childSpanID }
func (r *edgeResolver) ParentService() string { return r.item.parentService }
func (r *edgeResolver) ChildService() string  { return r.item.childService }
func (r *edgeResolver) Depth() int32          { return int32(r.item.depth) }

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boundedInt32(value uint64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func schemaErrors(items []*queryerrors.QueryError) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Message)
	}
	return result
}

func validUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }

func (r *traceResolver) RootCount() int32 { return boundedInt32(uint64(r.item.rootCount)) }

func (r *spanResolver) TraceID() string { return r.item.traceID }
