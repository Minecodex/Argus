-- The event's target_tool_id is provider-authored. Do not infer query identity
-- from model-supplied arguments or untrusted result content.
-- name: ListUndeliveredNativeQueryResults :many
SELECT call.id AS tool_call_id, artifact.result_ref,
       COALESCE(event.payload->>'target_tool_id','')::text AS target_tool_id
FROM tool_calls call
JOIN tool_results result ON result.tool_call_id=call.id AND result.enterprise_id=call.enterprise_id
JOIN artifacts artifact ON artifact.id=result.artifact_id AND artifact.enterprise_id=call.enterprise_id
JOIN runs run ON run.id=call.run_id AND run.enterprise_id=call.enterprise_id
JOIN conversation_events event ON event.run_id=call.run_id AND event.enterprise_id=call.enterprise_id
  AND event.event_type='tool_call_result' AND event.payload->>'tool_call_id'=call.id::text
WHERE call.run_id=$1 AND call.enterprise_id=$2 AND run.conversation_id=$3
  AND call.source='argus' AND call.status='succeeded' AND call.tool_id='tool.invoke'
  AND event.payload->>'target_tool_id' IN ('telemetry.promql.query','telemetry.kql.query','telemetry.skywalking.trace')
  AND NOT EXISTS (
    SELECT 1 FROM workspace_files file
    WHERE file.enterprise_id=call.enterprise_id AND file.conversation_id=run.conversation_id
      AND artifact.result_ref=ANY(file.source_result_refs) AND file.deleted_at IS NULL
      AND file.path='queries/' || CASE event.payload->>'target_tool_id'
        WHEN 'telemetry.promql.query' THEN 'metrics'
        WHEN 'telemetry.kql.query' THEN 'logs'
        WHEN 'telemetry.skywalking.trace' THEN 'traces' END || '/' || call.id::text || '.json'
  )
ORDER BY call.created_at,call.call_sequence;
