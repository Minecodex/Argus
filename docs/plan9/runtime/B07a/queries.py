"""Normal three-signal queries, matching the deployed application APM vocabulary."""
from datetime import datetime,timedelta,timezone

BUDGET={'max_scan_bytes':268435456,'max_rows':5000,'max_samples':1000000,'max_series':10000,'timeout_ms':10000,'max_result_bytes':8388608}
ROWS='sourceId sourceIds resourceId serviceId serviceNamespace serviceName instanceId instanceName operationName timestamp intervalSeconds observedSpanCount sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP50Ms durationP95Ms durationP99Ms'
COVERAGE='observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount unknownSourceCount duplicateObservationCount'
TRACE_FIELDS='sourceId resourceId rootPresent rootCount missingParentCount completeness traceId rootService rootOperation startTime duration spanCount errorCount status'
def document(view):
    if view=='traces':return 'query ApplicationAPM($serviceIds:[String!],$sourceId:String,$traceId:String,$operationName:String){queryTraces(pageSize:20,sourceId:$sourceId,traceId:$traceId,operationName:$operationName,filters:[{resource:true,key:"argus.service.id",values:$serviceIds}]){total traces{'+TRACE_FIELDS+'}}}'
    if view=='trace_detail':return 'query ApplicationAPM($traceId:String!,$sourceId:String!,$resourceId:String!){queryTrace(traceId:$traceId,sourceId:$sourceId,resourceId:$resourceId){'+TRACE_FIELDS+' spans{traceId sourceId resourceId resourceAttributes scopeName kind statusMessage traceState spanId parentSpanId serviceName operationName status startTime duration attributes events links} edges{missingParent cycle parentSpanId childSpanId parentService childService depth}}}'
    roots={'services':'queryAPMServices','instances':'queryAPMInstances','endpoints':'queryAPMEndpoints','red':'queryAPMRED','topology':'queryAPMTopology'}
    variables='$serviceIds:[String!]';args='limit:100,filters:[{resource:true,key:"argus.service.id",values:$serviceIds}]'
    if view=='red':variables+=',$bucketSeconds:Int!';args+=',bucketSeconds:$bucketSeconds'
    fields='basis status limited percentileMethod windowStart windowEnd coverage{'+COVERAGE+'} rows{'+ROWS+'}'
    if view=='topology':fields='basis status limited percentileMethod windowStart windowEnd coverage{observedSpanCount missingParentCount ambiguousParentCount cyclicSpanCount missingServiceCount unknownSourceCount} nodes{id resourceId sourceId serviceId serviceNamespace serviceName} edges{sourceNodeId targetNodeId sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP95Ms}'
    return 'query ApplicationAPM('+variables+'){'+roots[view]+'('+args+'){'+fields+'}}'
def scope(app,seconds=900):
    end=datetime.now(timezone.utc)
    return {'resource_ids':[app],'time_range':{'from':(end-timedelta(seconds=seconds)).isoformat(),'to':end.isoformat()},'budget':BUDGET}
def apm(api,app,view,variables=None):
    inputs={'serviceIds':None}
    if view=='red':inputs['bucketSeconds']=10
    inputs.update(variables or {})
    request=scope(app)
    if view=='topology':request['budget']={**BUDGET,'max_rows':50000}
    return api.call('POST','/enterprise/traces/graphql',{**request,'query':document(view),'operation_name':'ApplicationAPM','variables':inputs})
