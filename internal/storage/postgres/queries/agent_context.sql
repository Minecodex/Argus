-- name: ListCompactedRunUserContextEvents :many
-- User instructions remain authoritative ledger messages even when completed
-- tool groups in the same Run have moved into a historical summary.
SELECT * FROM conversation_events
WHERE conversation_id=$1 AND enterprise_id=$2 AND run_id=$3
  AND event_type='user_message' AND sequence <= sqlc.arg(through_sequence)::bigint
ORDER BY sequence;
