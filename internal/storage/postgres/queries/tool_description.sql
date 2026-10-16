-- name: HasEarlierNativeToolDescription :one
SELECT EXISTS (
 SELECT 1 FROM tool_calls d
 JOIN tool_results result ON result.tool_call_id=d.id AND result.enterprise_id=d.enterprise_id
 JOIN run_steps described_step ON described_step.id=d.step_id
 JOIN run_steps current_step ON current_step.id=sqlc.arg(step_id)::uuid
 JOIN runs r ON r.id=d.run_id
 WHERE d.enterprise_id=sqlc.arg(enterprise_id)::uuid
   AND r.actor_user_id=sqlc.arg(user_id)::uuid
   AND d.run_id=sqlc.arg(run_id)::uuid
   AND current_step.run_id=d.run_id AND current_step.enterprise_id=d.enterprise_id
   AND described_step.sequence<current_step.sequence
   AND d.model_call_id<>sqlc.arg(model_call_id)::uuid
   AND d.source='argus' AND d.tool_id='tool.describe' AND d.status='succeeded'
   AND d.input->>'category'=sqlc.arg(category)::text
   AND d.input->>'name'=sqlc.arg(name)::text
   AND result.projection->'summary'->>'version'=sqlc.arg(version)::text
);
