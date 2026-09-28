-- name: GetConversationDashboardContext :one
SELECT * FROM dashboard_conversation_contexts WHERE conversation_id=$1 AND enterprise_id=$2 AND owner_user_id=$3;

-- name: SetConversationDashboardContext :one
INSERT INTO dashboard_conversation_contexts(conversation_id,enterprise_id,owner_user_id,version,selection) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(conversation_id) DO UPDATE SET selection=EXCLUDED.selection,version=EXCLUDED.version,updated_at=now()
WHERE dashboard_conversation_contexts.enterprise_id=EXCLUDED.enterprise_id AND dashboard_conversation_contexts.owner_user_id=EXCLUDED.owner_user_id
RETURNING *;

-- name: CreateRunDashboardContext :one
INSERT INTO dashboard_run_contexts(run_id,enterprise_id,conversation_id,owner_user_id,context_version,selection) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: GetRunDashboardContext :one
SELECT * FROM dashboard_run_contexts WHERE run_id=$1 AND enterprise_id=$2 AND conversation_id=$3 AND owner_user_id=$4;
