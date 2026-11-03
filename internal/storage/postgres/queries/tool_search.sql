-- name: HasEarlierNativeToolSearch :one
SELECT EXISTS (
 SELECT 1 FROM tool_calls searched
 JOIN tool_results listing ON listing.tool_call_id=searched.id AND listing.enterprise_id=searched.enterprise_id
 JOIN run_steps search_step ON search_step.id=searched.step_id
 JOIN run_steps current_step ON current_step.id=sqlc.arg(step_id)::uuid
 JOIN runs r ON r.id=searched.run_id AND r.enterprise_id=searched.enterprise_id
 WHERE searched.enterprise_id=sqlc.arg(enterprise_id)::uuid
   AND r.actor_user_id=sqlc.arg(user_id)::uuid
   AND searched.run_id=sqlc.arg(run_id)::uuid
   AND current_step.run_id=searched.run_id AND current_step.enterprise_id=searched.enterprise_id
   AND search_step.sequence<current_step.sequence
   AND searched.model_call_id<>sqlc.arg(model_call_id)::uuid
   AND searched.source='argus' AND searched.tool_id='tool.search' AND searched.status='succeeded'
   AND searched.input->>'category'=sqlc.arg(category)::text
   AND listing.projection->'summary'->>'category'=sqlc.arg(category)::text
   AND listing.projection->'summary'->>'complete'='true'
   AND EXISTS (
     SELECT 1 FROM jsonb_array_elements(coalesce(listing.projection->'summary'->'items','[]'::jsonb)) item
     WHERE item->>'name'=sqlc.arg(name)::text AND item->>'version'=sqlc.arg(version)::text
   )
);
