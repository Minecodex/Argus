package skywalking

import "time"

// One grouped scan supplies both the untruncated coverage and the visible rows.
// Window totals are evaluated before dimension filtering and the explicit limit.
// An invalid-dimension sentinel preserves coverage when no group is displayable.
func apmWindowQuery(base, group string, bucket int32, start time.Time) (string, []any) {
	instance, instanceName, operation := "''", "''", "''"
	groups := "resource_id,source_id,service_name"
	visible := "service_name!=''"
	if group == "instances" {
		instance = "resource_attributes['service.instance.id']"
		instanceName = "argMax(resource_attributes['service.instance.name'],tuple(start_time,span_id))"
		groups += ",instance_id"
		visible += " AND instance_id!=''"
	}
	if group == "endpoints" {
		operation = "operation"
		groups += ",operation_name"
		visible += " AND operation_name!=''"
	}
	bucketSQL := "toDateTime64(?,9,'UTC')"
	values := []any{start}
	if group == "red" {
		bucketSQL = "fromUnixTimestamp64Nano(toUnixTimestamp64Nano(toDateTime64(?,9,'UTC'))+intDiv(toUnixTimestamp64Nano(start_time)-toUnixTimestamp64Nano(toDateTime64(?,9,'UTC')),?)*?, 'UTC')"
		nanos := int64(bucket) * int64(time.Second)
		values = []any{start, start, nanos, nanos}
		groups += ",bucket_start"
	}
	order := "bucket_start,source_id,resource_id,service_name,instance_id,operation_name"
	query := base + `, grouped AS (SELECT resource_id,source_id,service_name,` + instance + ` AS instance_id,` + instanceName + ` AS instance_name,` + operation + ` AS operation_name,` + bucketSQL + ` AS bucket_start,
	 count() AS observed,countIf(` + entrySpan + `) AS samples,countIf((` + entrySpan + `) AND status='error') AS errors,
	 avgIf(toFloat64(duration_ns)/1e6,` + entrySpan + `) AS mean_duration,quantilesTDigestIf(0.5,0.95,0.99)(toFloat64(duration_ns)/1e6,` + entrySpan + `) AS quantiles,
	 countIf(service_name='') AS missing_service,countIf(resource_attributes['service.instance.id']='') AS missing_instance,countIf(operation='') AS missing_operation,
	 countIf(source_id=toUUID('00000000-0000-0000-0000-000000000000') OR source_type IN ('','unknown')) AS unknown_source,maxOrNull(start_time) AS latest,
	 toUInt8(` + visible + `) AS visible
	 FROM samples GROUP BY ` + groups + `), covered AS (SELECT *,
	 sum(observed) OVER () AS total_observed,sum(samples) OVER () AS total_samples,
	 sum(missing_service) OVER () AS total_missing_service,sum(missing_instance) OVER () AS total_missing_instance,
	 sum(missing_operation) OVER () AS total_missing_operation,sum(unknown_source) OVER () AS total_unknown_source,
	 max(latest) OVER () AS latest_sample,countIf(visible=1) OVER () AS visible_groups,
	 row_number() OVER (ORDER BY visible DESC,` + order + `) AS position FROM grouped)
	 SELECT resource_id,source_id,service_name,instance_id,instance_name,operation_name,bucket_start,observed,samples,errors,mean_duration,quantiles,
	 total_observed,total_samples,total_missing_service,total_missing_instance,total_missing_operation,total_unknown_source,latest_sample,visible_groups,visible
	 FROM covered WHERE visible=1 OR position=1 ORDER BY visible DESC,` + order + ` LIMIT ?`
	return query, values
}
