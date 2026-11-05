package skywalking

import "fmt"

// ApplicationAPMQueries is the versioned portal query vocabulary. Schema
// validation and generation keep the portal aligned with the real engine.
func ApplicationAPMQueries() map[string]string {
	return applicationAPMQueries(100, true)
}

// ApplicationAPMQuery narrows a list request without increasing its execution
// budget. The managed portal vocabulary keeps its default page size of 100.
func ApplicationAPMQuery(view string, limit int) (string, error) {
	if limit < 1 || limit > 100 {
		return "", ErrBudget
	}
	query, ok := applicationAPMQueries(limit, false)[view]
	if !ok {
		return "", fmt.Errorf("unsupported application APM view")
	}
	return query, nil
}

func applicationAPMQueries(limit int, portal bool) map[string]string {
	rows := "sourceId sourceIds resourceId serviceId serviceNamespace serviceName instanceId instanceName operationName timestamp intervalSeconds observedSpanCount sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP50Ms durationP95Ms durationP99Ms"
	coverage := "observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount duplicateObservationCount"
	result := map[string]string{}
	for view, root := range map[string]string{"services": "queryAPMServices", "instances": "queryAPMInstances", "endpoints": "queryAPMEndpoints", "red": "queryAPMRED", "topology": "queryAPMTopology"} {
		variables := "$serviceIds:[String!]"
		limitValue := fmt.Sprint(limit)
		args := `filters:[{resource:true,key:"argus.service.id",values:$serviceIds}]`
		if view == "red" {
			variables += ",$bucketSeconds:Int!"
			args += ",bucketSeconds:$bucketSeconds"
			if portal {
				variables += ",$rowLimit:Int=100"
				limitValue = "$rowLimit"
			}
		}
		fields := "basis status limited percentileMethod windowStart windowEnd coverage{" + coverage + "} rows{" + rows + "}"
		if view == "topology" {
			fields = "basis status limited percentileMethod windowStart windowEnd coverage{observedSpanCount missingParentCount ambiguousParentCount cyclicSpanCount missingServiceCount unknownSourceCount} nodes{id resourceId sourceId serviceId serviceNamespace serviceName} edges{sourceNodeId targetNodeId sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP95Ms}"
		}
		result[view] = "query ApplicationAPM(" + variables + "){" + root + "(limit:" + limitValue + "," + args + "){" + fields + "}}"
	}
	traceSummaryFields := "sourceId resourceId rootPresent rootCount missingParentCount traceId rootService rootOperation startTime duration spanCount errorCount status"
	traceDetailFields := traceSummaryFields + " completeness"
	traceFields := traceDetailFields
	traceVars := "$serviceIds:[String!],$sourceId:String,$traceId:String,$operationName:String"
	traceArgs := fmt.Sprintf("pageSize:%d", limit)
	if portal {
		// Completeness resolves the Span graph. The portal list only displays the summary.
		traceFields = traceSummaryFields
		traceVars += ",$pageNum:Int,$pageSize:Int=100"
		traceArgs = "pageNum:$pageNum,pageSize:$pageSize"
	}
	result["traces"] = "query ApplicationAPM(" + traceVars + "){queryTraces(" + traceArgs + `,sourceId:$sourceId,traceId:$traceId,operationName:$operationName,filters:[{resource:true,key:"argus.service.id",values:$serviceIds}]){total traces{` + traceFields + "}}}"
	result["trace_detail"] = "query ApplicationAPM($traceId:String!,$sourceId:String!,$resourceId:String!){queryTrace(traceId:$traceId,sourceId:$sourceId,resourceId:$resourceId){" + traceDetailFields + " spans{traceId sourceId resourceId resourceAttributes scopeName kind statusMessage traceState spanId parentSpanId serviceName operationName status startTime duration attributes events links} edges{missingParent cycle parentSpanId childSpanId parentService childService depth}}}"
	return result
}
