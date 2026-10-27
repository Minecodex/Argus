-- name: CreateInstanceCollectionClaim :one
INSERT INTO collection_claims(id,enterprise_id,physical_resource_ref,collector_id,profile_id,collection_id,assignment_epoch,config_revision,
 claim_type,signal,selector,selector_hash,ownership,status,primary_claim_id,rollback_plan,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'active',$14,$15,$16) RETURNING *;
