package skywalking

// APM groups authenticated application services across collectors/protocols.
// Exact trace/span identities are deduplicated after source authorization. Raw
// trace views retain each source observation; unrelated span IDs are never
// guessed to be duplicate instrumentation. Legacy rows without an authenticated
// application/service identity retain their original source boundary.
func (state *executionState) apmFacts(where string) string {
	return `SELECT resource_id,trace_id,span_id,statistic_scope,
 latest.1 AS source_id,latest.2 AS parent_span_id,latest.3 AS service_name,latest.4 AS operation,
 if(countIf(observed.status='error')>0,'error',latest.5) AS status,
 latest.6 AS start_time,latest.7 AS end_time,latest.8 AS duration_ns,
 latest.9 AS attributes,latest.10 AS events,latest.11 AS links,latest.12 AS resource_attributes,
 latest.13 AS scope_name,latest.14 AS span_kind,latest.15 AS status_message,latest.16 AS trace_state,latest.17 AS source_type,
 groupUniqArray(toString(observed.source_id)) AS observation_source_ids,toUInt64(count()) AS observation_count,
 argMax(tuple(observed.source_id,observed.parent_span_id,observed.service_name,observed.operation,observed.status,
 observed.start_time,observed.end_time,observed.duration_ns,observed.attributes,observed.events,observed.links,
 observed.resource_attributes,observed.scope_name,observed.span_kind,observed.status_message,observed.trace_state,observed.source_type),
 tuple(observed.source_type='otlp',observed.resource_attributes['service.instance.id']!='',observed.end_time,observed.source_id)) AS latest
 FROM (SELECT *,if(resource_attributes['argus.service.id']!='' AND resource_attributes['argus.application.id']=toString(resource_id),
 concat('application:',resource_attributes['argus.service.id']),concat('source:',toString(source_id))) AS statistic_scope
 FROM (` + state.spanFacts(where) + `)) AS observed
 GROUP BY resource_id,trace_id,span_id,statistic_scope`
}
