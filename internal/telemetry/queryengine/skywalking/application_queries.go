package skywalking

import "fmt"

// ApplicationAPMQueries is the versioned portal query vocabulary. Schema
// validation and generation keep the portal aligned with the real engine.
func ApplicationAPMQueries() map[string]string {
	return applicationAPMQueries(100)
}

// ApplicationAPMQuery narrows a list request without increasing its execution
// budget. The managed portal vocabulary keeps its default page size of 100.
func ApplicationAPMQuery(view string, limit int) (string, error) {
	if limit < 1 || limit > 100 {
		return "", ErrBudget
	}
	query, ok := applicationAPMQueries(limit)[view]
	if !ok {
		return "", fmt.Errorf("unsupported application APM view")
	}
	return query, nil
}

func applicationAPMQueries(limit int) map[string]string {
	rows := "sourceId sourceIds resourceId serviceId serviceNamespace serviceName instanceId instanceName operationName timestamp intervalSeconds observedSpanCount sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP50Ms durationP95Ms durationP99Ms"
	coverage := "observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount duplicateObservationCount"
	result := map[string]string{}
	for view, root := range map[string]string{"services": "queryAPMServices", "instances": "queryAPMInstances", "endpoints": "queryAPMEndpoints", "red": "queryAPMRED", "topology": "queryAPMTopology"} {
		variables, args := "$serviceIds:[String!]", fmt.Sprintf("limit:%d,filters:[{resource:true,key:\"argus.service.id\",values:$serviceIds}]", limit)
		if view == "red" {
			variables += ",$bucketSeconds:Int!"
			args += ",bucketSeconds:$bucketSeconds"
		}
		fields := "basis status limited percentileMethod windowStart windowEnd coverage{" + coverage + "} rows{" + rows + "}"
		if view == "topology" {
			fields = "basis status limited percentileMethod windowStart windowEnd coverage{observedSpanCount missingParentCount ambiguousParentCount cyclicSpanCount missingServiceCount unknownSourceCount} nodes{id resourceId sourceId serviceId serviceNamespace serviceName} edges{sourceNodeId targetNodeId sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP95Ms}"
		}
		result[view] = "query ApplicationAPM(" + variables + "){" + root + "(" + args + "){" + fields + "}}"
	}
	traceFields := "sourceId resourceId rootPresent rootCount missingParentCount completeness traceId rootService rootOperation startTime duration spanCount errorCount status"
	result["traces"] = fmt.Sprintf("query ApplicationAPM($serviceIds:[String!],$sourceId:String,$traceId:String,$operationName:String){queryTraces(pageSize:%d,sourceId:$sourceId,traceId:$traceId,operationName:$operationName,filters:[{resource:true,key:\"argus.service.id\",values:$serviceIds}]){total traces{", limit) + traceFields + "}}}"
	result["trace_detail"] = "query ApplicationAPM($traceId:String!,$sourceId:String!,$resourceId:String!){queryTrace(traceId:$traceId,sourceId:$sourceId,resourceId:$resourceId){" + traceFields + " spans{traceId sourceId resourceId resourceAttributes scopeName kind statusMessage traceState spanId parentSpanId serviceName operationName status startTime duration attributes events links} edges{missingParent cycle parentSpanId childSpanId parentService childService depth}}}"
	return result
}
