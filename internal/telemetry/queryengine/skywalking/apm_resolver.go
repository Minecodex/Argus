package skywalking

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

type apmArgs struct {
	Filters                                                               *[]apmAttributeFilter
	ServiceName, ServiceInstanceName, OperationName, SourceID, ResourceID *string
	Limit                                                                 *int32
}
type apmREDArgs struct {
	Filters                                                               *[]apmAttributeFilter
	ServiceName, ServiceInstanceName, OperationName, SourceID, ResourceID *string
	Limit                                                                 *int32
	BucketSeconds                                                         int32
}

func (*rootResolver) QueryAPMServices(ctx context.Context, args apmArgs) (*apmResultResolver, error) {
	return resolveAPM(ctx, "services", args, 0)
}
func (*rootResolver) QueryAPMInstances(ctx context.Context, args apmArgs) (*apmResultResolver, error) {
	return resolveAPM(ctx, "instances", args, 0)
}
func (*rootResolver) QueryAPMEndpoints(ctx context.Context, args apmArgs) (*apmResultResolver, error) {
	return resolveAPM(ctx, "endpoints", args, 0)
}
func (*rootResolver) QueryAPMRED(ctx context.Context, args apmREDArgs) (*apmResultResolver, error) {
	return resolveAPM(ctx, "red", apmArgs{ServiceName: args.ServiceName, ServiceInstanceName: args.ServiceInstanceName, OperationName: args.OperationName, SourceID: args.SourceID, ResourceID: args.ResourceID, Limit: args.Limit, Filters: args.Filters}, args.BucketSeconds)
}

func resolveAPM(ctx context.Context, group string, args apmArgs, bucket int32) (*apmResultResolver, error) {
	if err := validateAPMArgs(group, args, bucket); err != nil {
		return nil, err
	}
	result := &apmResultResolver{group: group, rows: []*apmRowResolver{}}
	if ctx.Value(inputValidationKey{}) == true {
		return result, nil
	}
	state, err := executionStateFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if !state.request.End.After(state.request.Start) || state.request.End.Sub(state.request.Start) > 7*24*time.Hour {
		return nil, state.failed(fmt.Errorf("APM requires a time window within seven days"))
	}
	result, err = state.queryAPM(ctx, group, args, bucket)
	if err != nil {
		return nil, state.failed(err)
	}
	state.apmWarning(result.status)
	return result, nil
}

func validateAPMArgs(group string, args apmArgs, bucket int32) error {
	if err := validateAPMAttributes(args.Filters); err != nil {
		return err
	}
	for _, s := range []*string{args.ServiceName, args.ServiceInstanceName, args.OperationName} {
		if s != nil && len(*s) > 4096 {
			return fmt.Errorf("APM filter exceeds length budget")
		}
	}
	if err := validateTraceIdentity(args.SourceID); err != nil {
		return err
	}
	if err := validateTraceIdentity(args.ResourceID); err != nil {
		return err
	}
	if args.Limit != nil && (*args.Limit < 1 || *args.Limit > 50000) {
		return fmt.Errorf("invalid APM limit")
	}
	if group == "red" && (bucket < 1 || bucket > 86400) {
		return fmt.Errorf("APM time bucket must be 1-86400 seconds")
	}
	return nil
}

type apmResultResolver struct {
	group, status string
	start, end    time.Time
	limited       bool
	coverage      apmCoverageResolver
	rows          []*apmRowResolver
}

func (*apmResultResolver) Basis() string                    { return "received_entry_spans" }
func (*apmResultResolver) PercentileMethod() string         { return "tdigest" }
func (r *apmResultResolver) GroupBy() string                { return r.group }
func (r *apmResultResolver) Status() string                 { return r.status }
func (r *apmResultResolver) WindowStart() string            { return r.start.Format(time.RFC3339Nano) }
func (r *apmResultResolver) WindowEnd() string              { return r.end.Format(time.RFC3339Nano) }
func (r *apmResultResolver) Limited() bool                  { return r.limited }
func (r *apmResultResolver) Coverage() *apmCoverageResolver { return &r.coverage }
func (r *apmResultResolver) Rows() []*apmRowResolver        { return r.rows }

type apmCoverageResolver struct{ observed, requests, missingService, missingInstance, missingOperation, unknownSource uint64 }

func (r *apmCoverageResolver) ObservedSpanCount() float64     { return float64(r.observed) }
func (r *apmCoverageResolver) RequestSampleCount() float64    { return float64(r.requests) }
func (r *apmCoverageResolver) MissingServiceCount() float64   { return float64(r.missingService) }
func (r *apmCoverageResolver) MissingInstanceCount() float64  { return float64(r.missingInstance) }
func (r *apmCoverageResolver) MissingOperationCount() float64 { return float64(r.missingOperation) }
func (r *apmCoverageResolver) UnknownSourceCount() float64    { return float64(r.unknownSource) }

type apmRowResolver struct {
	sourceID, resourceID, service, instanceID, instanceName, operation string
	at                                                                 time.Time
	seconds                                                            float64
	observed, samples, errors                                          uint64
	mean                                                               float64
	quantiles                                                          []float64
}

func (r *apmRowResolver) SourceID() string           { return r.sourceID }
func (r *apmRowResolver) ResourceID() string         { return r.resourceID }
func (r *apmRowResolver) ServiceName() string        { return r.service }
func (r *apmRowResolver) InstanceID() string         { return r.instanceID }
func (r *apmRowResolver) InstanceName() string       { return r.instanceName }
func (r *apmRowResolver) OperationName() string      { return r.operation }
func (r *apmRowResolver) Timestamp() string          { return r.at.Format(time.RFC3339Nano) }
func (r *apmRowResolver) IntervalSeconds() float64   { return r.seconds }
func (r *apmRowResolver) ObservedSpanCount() float64 { return float64(r.observed) }
func (r *apmRowResolver) SampleCount() float64       { return float64(r.samples) }
func (r *apmRowResolver) ErrorCount() float64        { return float64(r.errors) }
func (r *apmRowResolver) ErrorRate() *float64 {
	return r.value(float64(r.errors) / float64(max(r.samples, 1)))
}
func (r *apmRowResolver) SamplesPerSecond() *float64 { return r.value(float64(r.samples) / r.seconds) }
func (r *apmRowResolver) DurationMeanMs() *float64   { return r.value(r.mean) }
func (r *apmRowResolver) DurationP50Ms() *float64    { return r.quantile(0) }
func (r *apmRowResolver) DurationP95Ms() *float64    { return r.quantile(1) }
func (r *apmRowResolver) DurationP99Ms() *float64    { return r.quantile(2) }
func (r *apmRowResolver) value(value float64) *float64 {
	if r.samples == 0 {
		return nil
	}
	return &value
}
func (r *apmRowResolver) quantile(index int) *float64 {
	if len(r.quantiles) <= index {
		return nil
	}
	return r.value(r.quantiles[index])
}

func (state *executionState) apmWarning(status string) {
	if status == "available" || status == "no_data" {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	code := "APM_" + strings.ToUpper(status)
	if !slices.Contains(state.warnings, code) {
		state.warnings = append(state.warnings, code)
	}
}
