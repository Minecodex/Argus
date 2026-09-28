-- name: GetDashboardParameterState :one
SELECT * FROM dashboard_parameter_states WHERE conversation_id=$1 AND dashboard_id=$2 AND enterprise_id=$3 AND owner_user_id=$4;

-- name: SetDashboardParameterState :one
INSERT INTO dashboard_parameter_states(enterprise_id,conversation_id,dashboard_id,owner_user_id,run_id,version,state) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(conversation_id,dashboard_id) DO UPDATE SET run_id=EXCLUDED.run_id,version=EXCLUDED.version,state=EXCLUDED.state
WHERE dashboard_parameter_states.enterprise_id=EXCLUDED.enterprise_id AND dashboard_parameter_states.owner_user_id=EXCLUDED.owner_user_id
 AND dashboard_parameter_states.version=sqlc.arg(expected_version)
RETURNING *;

-- name: CreateDashboardRunParameters :exec
INSERT INTO dashboard_run_parameters(run_id,enterprise_id,conversation_id,dashboard_id,owner_user_id,version,state) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: GetDashboardRunParameters :one
SELECT * FROM dashboard_run_parameters WHERE run_id=$1 AND dashboard_id=$2 AND enterprise_id=$3 AND owner_user_id=$4;

-- name: LockDashboardRunParameters :one
SELECT * FROM dashboard_run_parameters WHERE run_id=$1 AND dashboard_id=$2 AND enterprise_id=$3 AND owner_user_id=$4 FOR UPDATE;

-- name: SetDashboardRunParameters :one
UPDATE dashboard_run_parameters SET state=sqlc.arg(state),version=version+1
WHERE run_id=$1 AND dashboard_id=$2 AND enterprise_id=$3 AND owner_user_id=$4 AND version=sqlc.arg(expected_version)
RETURNING *;

-- name: CreateDashboardAnalysisContext :one
INSERT INTO dashboard_analysis_contexts(id,enterprise_id,conversation_id,run_id,dashboard_id,owner_user_id,revision_id,condition_version,input_hash,user_event_id,state,parameters)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: GetDashboardAnalysisContext :one
SELECT * FROM dashboard_analysis_contexts WHERE id=$1 AND enterprise_id=$2 AND conversation_id=$3 AND run_id=$4 AND owner_user_id=$5;

-- name: GetDashboardRunUserMessage :one
SELECT * FROM conversation_events WHERE run_id=$1 AND enterprise_id=$2 AND conversation_id=$3 AND event_type='user_message' ORDER BY sequence LIMIT 1;
