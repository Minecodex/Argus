package skywalking

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type topologyResult struct {
	status     string
	start, end time.Time
	limited    bool
	coverage   topologyCoverage
	nodes      []*topologyNode
	edges      []*topologyEdge
}

func (*topologyResult) Basis() string                 { return "received_destination_entry_spans" }
func (*topologyResult) PercentileMethod() string      { return "nearest_rank" }
func (r *topologyResult) Status() string              { return r.status }
func (r *topologyResult) WindowStart() string         { return r.start.Format(time.RFC3339Nano) }
func (r *topologyResult) WindowEnd() string           { return r.end.Format(time.RFC3339Nano) }
func (r *topologyResult) Limited() bool               { return r.limited }
func (r *topologyResult) Coverage() *topologyCoverage { return &r.coverage }
func (r *topologyResult) Nodes() []*topologyNode      { return r.nodes }
func (r *topologyResult) Edges() []*topologyEdge      { return r.edges }

type topologyCoverage struct{ observed, missing, ambiguous, cyclic, missingService, unknownSource int }

func (r *topologyCoverage) ObservedSpanCount() float64    { return float64(r.observed) }
func (r *topologyCoverage) MissingParentCount() float64   { return float64(r.missing) }
func (r *topologyCoverage) AmbiguousParentCount() float64 { return float64(r.ambiguous) }
func (r *topologyCoverage) CyclicSpanCount() float64      { return float64(r.cyclic) }
func (r *topologyCoverage) MissingServiceCount() float64  { return float64(r.missingService) }
func (r *topologyCoverage) UnknownSourceCount() float64   { return float64(r.unknownSource) }

type topologyNode struct{ id, source, resource, service string }

func (r *topologyNode) ID() string          { return r.id }
func (r *topologyNode) SourceID() string    { return r.source }
func (r *topologyNode) ResourceID() string  { return r.resource }
func (r *topologyNode) ServiceName() string { return r.service }

type topologyEdge struct {
	from, to  string
	count     uint64
	stats     apmRowResolver
	durations []float64
}

func (r *topologyEdge) SourceNodeID() string       { return r.from }
func (r *topologyEdge) TargetNodeID() string       { return r.to }
func (r *topologyEdge) ObservedEdgeCount() float64 { return float64(r.count) }
func (r *topologyEdge) SampleCount() float64       { return float64(r.stats.samples) }
func (r *topologyEdge) ErrorCount() float64        { return float64(r.stats.errors) }
func (r *topologyEdge) ErrorRate() *float64        { return r.stats.ErrorRate() }
func (r *topologyEdge) SamplesPerSecond() *float64 { return r.stats.SamplesPerSecond() }
func (r *topologyEdge) DurationMeanMs() *float64   { return r.stats.DurationMeanMs() }
func (r *topologyEdge) DurationP95Ms() *float64    { return r.stats.DurationP95Ms() }

type topologySpan struct {
	resource, source, sourceType, trace, id, parent, service, operation, status, instanceID, instanceName string
	kind                                                                                                  uint8
	duration                                                                                              uint64
}

func (s topologySpan) key() string       { return s.resource + "/" + s.source + "/" + s.trace + "/" + s.id }
func (s topologySpan) parentKey() string { return s.trace + "/" + s.parent }
func (s topologySpan) node() *topologyNode {
	hash := sha256.Sum256([]byte(s.resource + "\x00" + s.source + "\x00" + s.service))
	return &topologyNode{id: hex.EncodeToString(hash[:]), resource: s.resource, source: s.source, service: s.service}
}
