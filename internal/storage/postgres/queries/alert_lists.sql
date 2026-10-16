-- name: PageAlertInstances :many
SELECT * FROM alert_instances WHERE enterprise_id=sqlc.arg(enterprise_id) AND resource_ids <@ sqlc.arg(allowed_resources)::uuid[]
AND (sqlc.narg(query)::text IS NULL OR position(lower(sqlc.narg(query)::text) IN lower(concat_ws(' ',state->>'name',state->>'policy_name',state->>'metric',state->'labels',state->'members')))>0)
AND (sqlc.narg(policy_id)::uuid IS NULL OR policy_id=sqlc.narg(policy_id))
AND (sqlc.narg(resource_id)::uuid IS NULL OR sqlc.narg(resource_id)=ANY(resource_ids))
AND (sqlc.narg(severity)::text IS NULL OR severity=sqlc.narg(severity))
AND (sqlc.narg(lifecycle)::text IS NULL OR lifecycle=sqlc.narg(lifecycle))
AND (sqlc.narg(notification_state)::text IS NULL OR state->>'notification_state'=sqlc.narg(notification_state))
AND (sqlc.narg(acknowledged)::boolean IS NULL OR acknowledged=sqlc.narg(acknowledged))
AND (sqlc.narg(since)::timestamptz IS NULL OR last_evaluated_at>=sqlc.narg(since))
AND (sqlc.narg(until)::timestamptz IS NULL OR last_evaluated_at<=sqlc.narg(until))
AND (sqlc.narg(rule_id)::uuid IS NULL OR state->>'rule_id'=sqlc.narg(rule_id)::text OR EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(state->'members')='array' THEN state->'members' ELSE '[]'::jsonb END) member WHERE member->>'rule_id'=sqlc.narg(rule_id)::text))
AND (sqlc.narg(metric)::text IS NULL OR state->>'metric' LIKE '%'||sqlc.narg(metric)::text||'%' OR EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(state->'members')='array' THEN state->'members' ELSE '[]'::jsonb END) member WHERE member->>'expression' LIKE '%'||sqlc.narg(metric)::text||'%'))
AND ((sqlc.arg(history)::boolean AND (started_at IS NOT NULL OR lifecycle='pending')) OR (NOT sqlc.arg(history)::boolean AND lifecycle NOT IN ('resolved','closed') AND (lifecycle<>'normal' OR health<>'normal')))
AND (sqlc.narg(after_time)::timestamptz IS NULL OR (created_at,id)<(sqlc.narg(after_time),sqlc.narg(after_id)::uuid))
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_size);

-- name: CountFilteredAlertInstances :one
SELECT count(*) FROM alert_instances WHERE enterprise_id=sqlc.arg(enterprise_id) AND resource_ids <@ sqlc.arg(allowed_resources)::uuid[]
AND (sqlc.narg(query)::text IS NULL OR position(lower(sqlc.narg(query)::text) IN lower(concat_ws(' ',state->>'name',state->>'policy_name',state->>'metric',state->'labels',state->'members')))>0)
AND (sqlc.narg(policy_id)::uuid IS NULL OR policy_id=sqlc.narg(policy_id))
AND (sqlc.narg(resource_id)::uuid IS NULL OR sqlc.narg(resource_id)=ANY(resource_ids))
AND (sqlc.narg(severity)::text IS NULL OR severity=sqlc.narg(severity))
AND (sqlc.narg(lifecycle)::text IS NULL OR lifecycle=sqlc.narg(lifecycle))
AND (sqlc.narg(notification_state)::text IS NULL OR state->>'notification_state'=sqlc.narg(notification_state))
AND (sqlc.narg(acknowledged)::boolean IS NULL OR acknowledged=sqlc.narg(acknowledged))
AND (sqlc.narg(since)::timestamptz IS NULL OR last_evaluated_at>=sqlc.narg(since))
AND (sqlc.narg(until)::timestamptz IS NULL OR last_evaluated_at<=sqlc.narg(until))
AND (sqlc.narg(rule_id)::uuid IS NULL OR state->>'rule_id'=sqlc.narg(rule_id)::text OR EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(state->'members')='array' THEN state->'members' ELSE '[]'::jsonb END) member WHERE member->>'rule_id'=sqlc.narg(rule_id)::text))
AND (sqlc.narg(metric)::text IS NULL OR state->>'metric' LIKE '%'||sqlc.narg(metric)::text||'%' OR EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(state->'members')='array' THEN state->'members' ELSE '[]'::jsonb END) member WHERE member->>'expression' LIKE '%'||sqlc.narg(metric)::text||'%'))
AND ((sqlc.arg(history)::boolean AND (started_at IS NOT NULL OR lifecycle='pending')) OR (NOT sqlc.arg(history)::boolean AND lifecycle NOT IN ('resolved','closed') AND (lifecycle<>'normal' OR health<>'normal')));

-- name: PageAlertDeliveries :many
SELECT * FROM alert_deliveries WHERE enterprise_id=sqlc.arg(enterprise_id) AND resource_ids <@ sqlc.arg(allowed_resources)::uuid[]
AND (sqlc.narg(query)::text IS NULL OR position(lower(sqlc.narg(query)::text) IN lower(concat_ws(' ',payload->>'subject',payload->>'text',payload->>'event_type',last_error,status)))>0)
AND (sqlc.narg(channel_id)::uuid IS NULL OR channel_id=sqlc.narg(channel_id))
AND (sqlc.narg(policy_id)::uuid IS NULL OR payload->'policy_ids' @> to_jsonb(ARRAY[sqlc.narg(policy_id)::uuid]))
AND (sqlc.narg(resource_id)::uuid IS NULL OR sqlc.narg(resource_id)=ANY(resource_ids))
AND (sqlc.narg(notification_state)::text IS NULL OR status=sqlc.narg(notification_state))
AND (sqlc.narg(since)::timestamptz IS NULL OR created_at>=sqlc.narg(since))
AND (sqlc.narg(until)::timestamptz IS NULL OR created_at<=sqlc.narg(until))
AND (sqlc.narg(after_time)::timestamptz IS NULL OR (created_at,id)<(sqlc.narg(after_time),sqlc.narg(after_id)::uuid))
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_size);

-- name: CountFilteredAlertDeliveries :one
SELECT count(*) FROM alert_deliveries WHERE enterprise_id=sqlc.arg(enterprise_id) AND resource_ids <@ sqlc.arg(allowed_resources)::uuid[]
AND (sqlc.narg(query)::text IS NULL OR position(lower(sqlc.narg(query)::text) IN lower(concat_ws(' ',payload->>'subject',payload->>'text',payload->>'event_type',last_error,status)))>0)
AND (sqlc.narg(channel_id)::uuid IS NULL OR channel_id=sqlc.narg(channel_id))
AND (sqlc.narg(policy_id)::uuid IS NULL OR payload->'policy_ids' @> to_jsonb(ARRAY[sqlc.narg(policy_id)::uuid]))
AND (sqlc.narg(resource_id)::uuid IS NULL OR sqlc.narg(resource_id)=ANY(resource_ids))
AND (sqlc.narg(notification_state)::text IS NULL OR status=sqlc.narg(notification_state))
AND (sqlc.narg(since)::timestamptz IS NULL OR created_at>=sqlc.narg(since))
AND (sqlc.narg(until)::timestamptz IS NULL OR created_at<=sqlc.narg(until));
