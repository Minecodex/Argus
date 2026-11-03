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

-- Bounded current-Run public evidence metadata. target_tool_id is authored by
-- the Harness; external MCP data and model-authored paths cannot enter it.
-- name: ListRunNativeQueryEvidence :many
SELECT call.id AS tool_call_id, artifact.result_ref, artifact.content_hash,
       artifact.byte_size, artifact.authorization_scope,
       result.projection,
       EXISTS(SELECT 1 FROM model_calls model
         WHERE model.enterprise_id=call.enterprise_id AND model.run_id=call.run_id
         AND model.call_kind='inference' AND model.status='succeeded' AND model.dispatched_at IS NOT NULL
         AND model.inline_query_receipts @> jsonb_build_array(jsonb_build_object(
           'query_call_id',call.id::text,'original_sha256',encode(artifact.content_hash,'hex'),
           'original_bytes',artifact.byte_size,'provider_request_hash',encode(model.projection_hash,'hex')))) AS inline_exposure_verified,
       (event.payload->>'target_tool_id')::text AS target_tool_id,
       COALESCE(file.path,'')::text AS file_path,
       COALESCE(file.content_hash,'')::text AS file_hash,
       COALESCE(file.byte_size,0)::bigint AS file_bytes
FROM tool_calls call
JOIN tool_results result ON result.tool_call_id=call.id AND result.enterprise_id=call.enterprise_id
JOIN artifacts artifact ON artifact.id=result.artifact_id AND artifact.enterprise_id=call.enterprise_id
JOIN runs run ON run.id=call.run_id AND run.enterprise_id=call.enterprise_id
JOIN conversation_events event ON event.run_id=call.run_id AND event.enterprise_id=call.enterprise_id
 AND event.event_type='tool_call_result' AND event.payload->>'tool_call_id'=call.id::text
LEFT JOIN workspaces workspace ON workspace.conversation_id=run.conversation_id
 AND workspace.enterprise_id=run.enterprise_id AND workspace.status='ready'
LEFT JOIN workspace_files file ON file.workspace_id=workspace.id AND file.enterprise_id=call.enterprise_id
 AND file.conversation_id=run.conversation_id AND file.deleted_at IS NULL
 AND artifact.result_ref=ANY(file.source_result_refs)
 AND file.path='queries/' || CASE event.payload->>'target_tool_id'
   WHEN 'telemetry.promql.query' THEN 'metrics'
   WHEN 'telemetry.kql.query' THEN 'logs'
   WHEN 'telemetry.skywalking.trace' THEN 'traces' END || '/' || call.id::text || '.json'
WHERE call.run_id=$1 AND call.enterprise_id=$2 AND run.conversation_id=$3
 AND call.source='argus' AND call.status='succeeded' AND call.tool_id='tool.invoke'
 AND artifact.authorization_scope=sqlc.arg(authorization_scope)::text
 AND event.payload->>'target_tool_id' IN ('telemetry.promql.query','telemetry.kql.query','telemetry.skywalking.trace')
ORDER BY call.created_at,call.call_sequence LIMIT 65;

-- Receipt bytes are computed from the actual assembled current request and
-- committed in its dispatch transaction. Unknown/failed requests never certify
-- historical exposure. Old rows remain empty; never backfill from old flags.
-- name: SetModelInlineQueryReceipts :exec
UPDATE model_calls SET inline_query_receipts=$3::jsonb
WHERE id=$1 AND enterprise_id=$2 AND status='running' AND dispatched_at IS NOT NULL;

-- A full read is evidence only after the caller checks content against the
-- immutable hash/length. Bash success alone does not establish consumption.
-- name: ListRunQueryFileReads :many
SELECT call.id, call.input, result.projection
FROM tool_calls call JOIN tool_results result
 ON result.tool_call_id=call.id AND result.enterprise_id=call.enterprise_id
WHERE call.run_id=$1 AND call.enterprise_id=$2 AND call.source='sandbox'
 AND call.tool_id='read' AND call.status='succeeded'
 AND call.authorization_scope=sqlc.arg(authorization_scope)::text
 AND call.input->>'path'=ANY(sqlc.arg(paths)::text[])
ORDER BY call.created_at,call.call_sequence LIMIT 65;
